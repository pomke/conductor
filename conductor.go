package conductor

import (
	"context"
	"log"
	"sync"
	"time"
)

const (
	startupTimeout  time.Duration = time.Duration(5 * time.Second)
	shutdownTimeout time.Duration = time.Duration(5 * time.Second)
)

type Service interface {
	Run(chan bool, chan bool, chan context.Context) error
}

type serviceState struct {
	name     string
	service  Service
	ready    chan bool
	stopped  chan bool
	shutdown chan context.Context
}

type conductor struct {
	started      bool          // Have we been started yet?
	noisy        bool          // Should we log?
	startTimeout time.Duration // How long should we wait for each service to start before we die?
	stopTimeout  time.Duration // How long should we wait for each service to stop before we kill it?
	shutdown     chan bool     // channel to block on, indicates everything has stopped, returned from Start()
	services     []*serviceState
}

/* Create a new conductor instance, accepts Option funcs (see README.md)
for changing default behaviours */
func New(opts ...func(*conductor)) *conductor {
	c := conductor{
		started:      false,
		noisy:        false,
		startTimeout: startupTimeout,
		stopTimeout:  shutdownTimeout,
		shutdown:     make(chan bool),
		services:     []*serviceState{},
	}

	for _, optFn := range opts {
		optFn(&c)
	}
	return &c
}

/* Add a Service with a name to be started in order when Start is called */
func (c *conductor) Service(name string, service Service) {
	if c.started {
		panic("Cannot call Conductor.Service after Conductor.Start")
	}
	c.services = append(c.services,
		&serviceState{name, service, make(chan bool, 1), make(chan bool, 1), make(chan context.Context, 1)})
}

/* Start the conductor, each service is started in turn */
func (c *conductor) Start() chan bool {
	c.started = true

	// start each ManagedService one at a time, this gives us service dependency order.
SRV_LOOP:
	for _, srv := range c.services {
		c.log("Starting service: ", srv.name)
		err := srv.service.Run(srv.ready, srv.stopped, srv.shutdown)
		if err != nil {
			// Service has failed to start with an error, shutdown everything
			c.logf("Service '%s' exited with: %s", srv.name, err)
			c.Stop()
			break
		}
		select {
		case <-time.After(c.startTimeout):
			// Service has timed out, shutdown everything
			c.logf("Service timed-out during startup %s", srv.name)
			c.Stop()
			break SRV_LOOP
		case <-srv.ready:
			// Service started up ok!
			c.log(srv.name, ".. ok")
			continue
		}
	}
	return c.shutdown
}

// stop the conductor, begin shutting down services
func (c *conductor) Stop() {
	// signal all services they should shutdown within timeout seconds
	ctx, _ := context.WithTimeout(context.Background(), c.stopTimeout)

	wg := sync.WaitGroup{}
	// we're waiting for this many services to close..
	wg.Add(len(c.services))

	// create a done channel that gets closed when all services are shutdown
	done := make(chan bool)
	go func() {
		wg.Wait()
		done <- true
	}()

	// decrement our waitgroup when each service says it has stopped
	for _, state := range c.services {
		state.shutdown <- ctx
		go func() {
			<-state.stopped
			wg.Done()
		}()
	}

	// Wait for either all services to close, or the timeout to occur then signal shutdown.
	select {
	case <-done:
		close(c.shutdown)
		return
		/*
			case <-time.After(c.stopTimeout + time.Second):
				c.log("Timeout exeeded waiting for services to stop, shutting down")
				cancel()
				close(c.shutdown)
				return*/
	}
}

func (c *conductor) logf(s string, v ...interface{}) {
	if c.noisy {
		log.Printf(s, v...)
	}
}

func (c *conductor) log(v ...interface{}) {
	if c.noisy {
		log.Print(v...)
	}
}
