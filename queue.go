package payloadqueue

import (
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Queue to hold the main application queuing mechanism.
type Queue struct {
	Tag          string
	MaxSize      int
	MaxAge       int // seconds
	Work         workHandler
	EventFeed    eventFeed
	payloadMutex sync.Mutex
	payloadQueue []Payload
	payloadChan  chan Payload
	quitChan     chan bool
	expires      time.Time
	activeWork   int // holds the number of active work routines that have not been completed.
}

// Start to open the queue to receive payload to batch
func (q *Queue) Start() error {
	q.expires = time.Now().Add(time.Duration(q.MaxAge) * time.Second)
	if q.Work == nil {
		return errors.New("the Work function is not supplied")
	}
	if q.MaxSize == 0 {
		q.MaxSize = 100
		q.event("MaxSize: Default value of 100 was used")
	}
	if q.MaxAge == 0 {
		q.MaxAge = 10
		q.event("MaxAge: Default value of 10 was used")
	}
	if q.Tag == "" {
		q.Tag = defaultTag(12)
		q.event("Tag: Random value assigned is: " + q.Tag)
	}
	q.activeWork = 0

	go func() {
		// Check for the max age
		for {
			time.Sleep(2 * time.Second)
			if time.Now().After(q.expires) {
				q.Append(Payload{})
			}
		}
	}()

	go func() {
		for {
			select {
			case p := <-q.payloadChan:
				// Payload has been added for queuing
				q.Append(p)

			case <-q.quitChan:
				// We have been asked to stop.
				q.Close()
				return
			}
		}
	}()
	q.event("BP Queue: Started")
	return nil
}

func (q Queue) NewPayload(pl interface{}) Payload {
	if pl == nil {
		return Payload{
			Id:   "",
			Data: "",
		}
	}
	u := uuid.New()
	return Payload{
		Id:   u.String(),
		Data: pl,
	}
}

// Run to push the Batch for processing
func (q *Queue) Run(Payloads []Payload) error {
	if q.Work == nil {
		return errors.New("no Work() is passed")
	}
	q.event("Batch Push [" + q.Tag + "]: Running. Queue Size: " + strconv.Itoa(len(Payloads)) + " @ " + time.Now().String())
	q.activeWork++
	pl := make([]interface{}, 0)
	for _, v := range Payloads {
		pl = append(pl, v.Data)
	}
	result := q.Work(pl)
	q.event("Batch Push [" + q.Tag + "]: Finished. Result Code: " + strconv.Itoa(result) + " @ " + time.Now().String())
	q.activeWork--

	return nil
}

// Append to add a Payload to the queue. This is a
func (q *Queue) Append(p Payload) error {
	// Add to the queue
	if p.Id != "" {
		q.payloadMutex.Lock()
		q.payloadQueue = append(q.payloadQueue, p)
		q.payloadMutex.Unlock()
		q.event("Payload Queued [id]: " + p.Id)
	}
	// Check the conditions for firing the Work()
	// 1. Queue is full
	// 2. MaxAge has expired
	if len(q.payloadQueue) >= q.MaxSize || time.Now().After(q.expires) {
		q.payloadMutex.Lock()
		pls := q.payloadQueue
		go q.Run(pls)
		// reset the queue
		q.payloadQueue = nil
		q.payloadMutex.Unlock()
		q.expires = time.Now().Add(time.Duration(q.MaxAge) * time.Second)
	}
	return nil
}

// Close to close the channels and wait for Work funcs to quit the execution.
func (q *Queue) Close() {
	q.event("Buffer Queue: Stopping...")
	if q.payloadChan != nil {
		close(q.payloadChan)
	}
	if q.quitChan != nil {
		close(q.quitChan)
	}
	// wait for all active routines to be completed
	for q.activeWork > 0 {
		time.Sleep(time.Second * 1)
	}
	q.event("Buffer Queue: All Work completed")
}

// event to write events into the Queue's feed
func (q *Queue) event(s string) {
	if q.EventFeed != nil {
		q.EventFeed("[" + q.Tag + "] " + s)
	}
}

// Size to return the number of payloads in the queue
func (q *Queue) Size() int {
	return len(q.payloadQueue)
}
