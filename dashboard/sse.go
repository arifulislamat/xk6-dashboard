// SPDX-FileCopyrightText: 2021 - 2023 Iván Szkiba
//
// SPDX-License-Identifier: MIT

package dashboard

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/r3labs/sse/v2"
	"github.com/sirupsen/logrus"
)

type eventEmitter struct {
	*sse.Server
	logger  logrus.FieldLogger
	channel string
	wait    sync.WaitGroup
	mu      sync.RWMutex
	count   int
	id      atomic.Int64
}

var _ eventListener = (*eventEmitter)(nil)

func newEventEmitter(channel string, logger logrus.FieldLogger) *eventEmitter {
	emitter := &eventEmitter{ //nolint:exhaustruct
		channel: channel,
		logger:  logger,
		Server:  sse.New(),
	}

	emitter.Server.OnSubscribe = emitter.onSubscribe
	emitter.Server.OnUnsubscribe = emitter.onUnsubscribe

	emitter.CreateStream(channel)

	return emitter
}

func (emitter *eventEmitter) onSubscribe(_ string, _ *sse.Subscriber) {
	emitter.mu.Lock()
	defer emitter.mu.Unlock()
	emitter.count++
	emitter.wait.Add(1)
}

func (emitter *eventEmitter) onUnsubscribe(_ string, _ *sse.Subscriber) {
	emitter.mu.Lock()
	defer emitter.mu.Unlock()

	emitter.count--

	if emitter.count >= 0 { // it seem onUnsubscribe sometimes called without onSubscribe...
		emitter.wait.Done()
	}
}

func (emitter *eventEmitter) onStart() error {
	return nil
}

func (emitter *eventEmitter) onStop() error {
	emitter.mu.RLock()
	defer emitter.mu.RUnlock()
	emitter.wait.Wait()

	return nil
}

func (emitter *eventEmitter) onEvent(name string, data interface{}) {
	buff, err := json.Marshal(data)
	if err != nil {
		emitter.logger.Error(err)

		return
	}

	var retry []byte

	if name == stopEvent {
		retry = []byte(strconv.Itoa(maxSafeInteger))
	}

	id := strconv.FormatInt(emitter.id.Add(1), 10)

	emitter.Publish(
		emitter.channel,
		&sse.Event{Event: []byte(name), Data: buff, Retry: retry, ID: []byte(id)},
	) //nolint:exhaustruct
}

func (emitter *eventEmitter) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	values := req.URL.Query()

	values.Add("stream", emitter.channel)
	req.URL.RawQuery = values.Encode()

	res.Header().Set("Access-Control-Allow-Origin", "*")

	emitter.Server.ServeHTTP(res, req)
}

const maxSafeInteger = 9007199254740991
