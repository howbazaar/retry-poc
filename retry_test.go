// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package retry_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/retry"
)

type retrySuite struct {
	testing.LoggingSuite
}

var _ = gc.Suite(&retrySuite{})

type mockClock struct {
	delays []time.Duration
}

func (*mockClock) Now() time.Time {
	return time.Now()
}

func (mock *mockClock) After(wait time.Duration) <-chan time.Time {
	mock.delays = append(mock.delays, wait)
	return time.After(time.Microsecond)
}

func (*retrySuite) TestSuccessHasNoDelay(c *gc.C) {
	clock := &mockClock{}
	err := retry.Call(retry.CallArgs{
		Func:     func() error { return nil },
		Attempts: 5,
		Delay:    time.Minute,
		Clock:    clock,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clock.delays, gc.HasLen, 0)
}

func (*retrySuite) TestCalledOnceEvenIfStopped(c *gc.C) {
	stop := make(chan struct{})
	clock := &mockClock{}
	called := false
	close(stop)
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			called = true
			return nil
		},
		Attempts: 5,
		Delay:    time.Minute,
		Clock:    clock,
		Stop:     stop,
	})
	c.Assert(called, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clock.delays, gc.HasLen, 0)
}

func (*retrySuite) TestAttempts(c *gc.C) {
	clock := &mockClock{}
	funcErr := errors.New("bah")
	err := retry.Call(retry.CallArgs{
		Func:     func() error { return funcErr },
		Attempts: 4,
		Delay:    time.Minute,
		Clock:    clock,
	})
	c.Assert(errors.Cause(err), jc.Satisfies, retry.IsAttemptsExceeded)
	// We delay between attempts, and don't delay after the last one.
	c.Assert(clock.delays, jc.DeepEquals, []time.Duration{
		time.Minute,
		time.Minute,
		time.Minute,
	})
}

func (*retrySuite) TestAttemptsExceededError(c *gc.C) {
	clock := &mockClock{}
	funcErr := errors.New("bah")
	err := retry.Call(retry.CallArgs{
		Func:     func() error { return funcErr },
		Attempts: 5,
		Delay:    time.Minute,
		Clock:    clock,
	})
	c.Assert(err, gc.ErrorMatches, `attempt count exceeded: bah`)
	cause := errors.Cause(err)
	c.Assert(cause, jc.Satisfies, retry.IsAttemptsExceeded)
	retryError, _ := cause.(*retry.AttemptsExceeded)
	c.Assert(retryError.LastError, gc.Equals, funcErr)
}

func (*retrySuite) TestFatalErrorsNotRetried(c *gc.C) {
	clock := &mockClock{}
	funcErr := errors.New("bah")
	err := retry.Call(retry.CallArgs{
		Func:         func() error { return funcErr },
		IsFatalError: func(error) bool { return true },
		Attempts:     5,
		Delay:        time.Minute,
		Clock:        clock,
	})
	c.Assert(errors.Cause(err), gc.Equals, funcErr)
	c.Assert(clock.delays, gc.HasLen, 0)
}

func (*retrySuite) TestBackoffFactor(c *gc.C) {
	clock := &mockClock{}
	err := retry.Call(retry.CallArgs{
		Func:          func() error { return errors.New("bah") },
		Clock:         clock,
		Attempts:      5,
		Delay:         time.Minute,
		BackoffFactor: 2,
	})
	c.Assert(errors.Cause(err), jc.Satisfies, retry.IsAttemptsExceeded)
	c.Assert(clock.delays, jc.DeepEquals, []time.Duration{
		time.Minute,
		time.Minute * 2,
		time.Minute * 4,
		time.Minute * 8,
	})
}

func (*retrySuite) TestStopChannel(c *gc.C) {
	clock := &mockClock{}
	stop := make(chan struct{})
	count := 0
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			if count == 2 {
				close(stop)
			}
			count++
			return errors.New("bah")
		},
		Attempts: 5,
		Delay:    time.Minute,
		Clock:    clock,
		Stop:     stop,
	})
	c.Assert(errors.Cause(err), jc.Satisfies, retry.IsRetryStopped)
	c.Assert(clock.delays, gc.HasLen, 3)
}

func (*retrySuite) TestNotifyFunc(c *gc.C) {
	var (
		clock      = &mockClock{}
		funcErr    = errors.New("bah")
		attempts   []int
		funcErrors []error
	)
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			return funcErr
		},
		NotifyFunc: func(lastError error, attempt int) {
			funcErrors = append(funcErrors, lastError)
			attempts = append(attempts, attempt)
		},
		Attempts: 3,
		Delay:    time.Minute,
		Clock:    clock,
	})
	c.Assert(errors.Cause(err), jc.Satisfies, retry.IsAttemptsExceeded)
	c.Assert(clock.delays, gc.HasLen, 2)
	c.Assert(funcErrors, jc.DeepEquals, []error{funcErr, funcErr, funcErr})
	c.Assert(attempts, jc.DeepEquals, []int{1, 2, 3})
}

func (*retrySuite) TestInfiniteRetries(c *gc.C) {
	// OK, we can't test infinite, but we'll go for lots.
	clock := &mockClock{}
	stop := make(chan struct{})
	count := 0
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			if count == 111 {
				close(stop)
			}
			count++
			return errors.New("bah")
		},
		Attempts: retry.UnlimitedAttempts,
		Delay:    time.Minute,
		Clock:    clock,
		Stop:     stop,
	})
	c.Assert(errors.Cause(err), jc.Satisfies, retry.IsRetryStopped)
	c.Assert(clock.delays, gc.HasLen, count)
}

func (*retrySuite) TestMaxDelay(c *gc.C) {
	clock := &mockClock{}
	err := retry.Call(retry.CallArgs{
		Func:          func() error { return errors.New("bah") },
		Attempts:      7,
		Delay:         time.Minute,
		MaxDelay:      10 * time.Minute,
		BackoffFactor: 2,
		Clock:         clock,
	})
	c.Assert(errors.Cause(err), jc.Satisfies, retry.IsAttemptsExceeded)
	c.Assert(clock.delays, jc.DeepEquals, []time.Duration{
		time.Minute,
		2 * time.Minute,
		4 * time.Minute,
		8 * time.Minute,
		10 * time.Minute,
		10 * time.Minute,
	})
}

func (*retrySuite) TestWithWallClock(c *gc.C) {
	var attempts []int
	err := retry.Call(retry.CallArgs{
		Func: func() error { return errors.New("bah") },
		NotifyFunc: func(lastError error, attempt int) {
			attempts = append(attempts, attempt)
		},
		Attempts: 5,
		Delay:    time.Microsecond,
	})
	c.Assert(errors.Cause(err), jc.Satisfies, retry.IsAttemptsExceeded)
	c.Assert(attempts, jc.DeepEquals, []int{1, 2, 3, 4, 5})
}

func (*retrySuite) TestMissingFuncNotValid(c *gc.C) {
	err := retry.Call(retry.CallArgs{
		Attempts: 5,
		Delay:    time.Minute,
	})
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `missing Func not valid`)
}

func (*retrySuite) TestMissingAttemptsNotValid(c *gc.C) {
	err := retry.Call(retry.CallArgs{
		Func:  func() error { return errors.New("bah") },
		Delay: time.Minute,
	})
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `missing Attempts not valid`)
}

func (*retrySuite) TestMissingDelayNotValid(c *gc.C) {
	err := retry.Call(retry.CallArgs{
		Func:     func() error { return errors.New("bah") },
		Attempts: 5,
	})
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `missing Delay not valid`)
}

func (*retrySuite) TestBackoffErrors(c *gc.C) {
	// Backoff values of less than one are a validation error.
	for _, factor := range []float64{-2, 0.5} {
		err := retry.Call(retry.CallArgs{
			Func:          func() error { return errors.New("bah") },
			Attempts:      5,
			Delay:         time.Minute,
			BackoffFactor: factor,
		})
		c.Check(err, jc.Satisfies, errors.IsNotValid)
		c.Check(err, gc.ErrorMatches, `BackoffFactor of .* not valid`)
	}
}

func (*retrySuite) TestCallArgsDefaults(c *gc.C) {
	// BackoffFactor is one of the two values with reasonable
	// defaults, and the default is linear if not specified.
	// The other default is the Clock. If not specified, it is the
	// wall clock.
	args := retry.CallArgs{
		Func:     func() error { return errors.New("bah") },
		Attempts: 5,
		Delay:    time.Minute,
	}

	err := args.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(args.BackoffFactor, gc.Equals, float64(1))
	c.Assert(args.Clock, gc.Equals, clock.WallClock)
}

func (*retrySuite) TestScaleDuration(c *gc.C) {
	for i, test := range []struct {
		current time.Duration
		max     time.Duration
		scale   float64
		expect  time.Duration
	}{{
		current: time.Minute,
		scale:   1,
		expect:  time.Minute,
	}, {
		current: time.Minute,
		scale:   2.5,
		expect:  2*time.Minute + 30*time.Second,
	}, {
		current: time.Minute,
		max:     3 * time.Minute,
		scale:   10,
		expect:  3 * time.Minute,
	}, {
		current: time.Minute,
		max:     3 * time.Minute,
		scale:   2,
		expect:  2 * time.Minute,
	}, {
		// scale factors of < 1 are not passed in from the Call function
		// but are supported by ScaleDuration
		current: time.Minute,
		scale:   0.5,
		expect:  30 * time.Second,
	}, {
		current: time.Minute,
		scale:   0,
		expect:  0,
	}, {
		// negative scales are treated as positive
		current: time.Minute,
		scale:   -2,
		expect:  2 * time.Minute,
	}} {
		c.Logf("test %d", i)
		c.Check(retry.ScaleDuration(test.current, test.max, test.scale), gc.Equals, test.expect)
	}
}
