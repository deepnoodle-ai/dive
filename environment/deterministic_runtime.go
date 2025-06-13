package environment

import (
	"context"
	"crypto/rand"
	"math/big"
	"sync/atomic"
	"time"
)

// DeterministicRuntime provides deterministic access to time and random values
// through the operation system, enabling reliable replay
type DeterministicRuntime struct {
	execution    *EventBasedExecution
	callSequence int64 // Counter for generating unique operation IDs
}

// NewDeterministicRuntime creates a new deterministic runtime
func NewDeterministicRuntime(execution *EventBasedExecution) *DeterministicRuntime {
	return &DeterministicRuntime{
		execution:    execution,
		callSequence: 0,
	}
}

// getNextCallSequence returns the next call sequence number
func (d *DeterministicRuntime) getNextCallSequence() int64 {
	return atomic.AddInt64(&d.callSequence, 1)
}

// Now provides deterministic access to current time
func (d *DeterministicRuntime) Now() time.Time {
	seq := d.getNextCallSequence()
	op := NewOperation(
		"time_access",
		"", // No specific step
		d.execution.currentPathID,
		map[string]interface{}{
			"access_type":   "now",
			"call_sequence": seq,
		},
	)

	timeInterface, err := d.execution.ExecuteOperation(context.Background(), op, func() (interface{}, error) {
		return time.Now(), nil
	})

	if err != nil {
		// This should never happen for time access, but if it does, panic
		// since deterministic execution requires all operations to succeed on replay
		panic("time access operation failed: " + err.Error())
	}

	return timeInterface.(time.Time)
}

// Unix provides deterministic access to Unix timestamp
func (d *DeterministicRuntime) Unix() int64 {
	return d.Now().Unix()
}

// UnixNano provides deterministic access to Unix nanosecond timestamp
func (d *DeterministicRuntime) UnixNano() int64 {
	return d.Now().UnixNano()
}

// Random provides deterministic access to random float64 values
func (d *DeterministicRuntime) Random() float64 {
	seq := d.getNextCallSequence()
	op := NewOperation(
		"random_generation",
		"",
		d.execution.currentPathID,
		map[string]interface{}{
			"type":          "float64",
			"call_sequence": seq,
		},
	)

	randInterface, err := d.execution.ExecuteOperation(context.Background(), op, func() (interface{}, error) {
		// Use crypto/rand for better randomness
		n, err := rand.Int(rand.Reader, big.NewInt(1<<53))
		if err != nil {
			return 0.0, err
		}
		return float64(n.Int64()) / float64(1<<53), nil
	})

	if err != nil {
		panic("random generation operation failed: " + err.Error())
	}

	return randInterface.(float64)
}

// RandomInt provides deterministic access to random integers within a range
func (d *DeterministicRuntime) RandomInt(min, max int64) int64 {
	if min >= max {
		panic("invalid range: min must be less than max")
	}

	seq := d.getNextCallSequence()
	op := NewOperation(
		"random_generation",
		"",
		d.execution.currentPathID,
		map[string]interface{}{
			"type":          "int64",
			"min":           min,
			"max":           max,
			"call_sequence": seq,
		},
	)

	randInterface, err := d.execution.ExecuteOperation(context.Background(), op, func() (interface{}, error) {
		rangeBig := big.NewInt(max - min)
		n, err := rand.Int(rand.Reader, rangeBig)
		if err != nil {
			return int64(0), err
		}
		return n.Int64() + min, nil
	})

	if err != nil {
		panic("random int generation operation failed: " + err.Error())
	}

	return randInterface.(int64)
}

// RandomString generates a deterministic random string of specified length
func (d *DeterministicRuntime) RandomString(length int, charset string) string {
	if charset == "" {
		charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	}

	seq := d.getNextCallSequence()
	op := NewOperation(
		"random_generation",
		"",
		d.execution.currentPathID,
		map[string]interface{}{
			"type":          "string",
			"length":        length,
			"charset":       charset,
			"call_sequence": seq,
		},
	)

	strInterface, err := d.execution.ExecuteOperation(context.Background(), op, func() (interface{}, error) {
		result := make([]byte, length)
		charsetBytes := []byte(charset)
		charsetLen := big.NewInt(int64(len(charsetBytes)))

		for i := 0; i < length; i++ {
			n, err := rand.Int(rand.Reader, charsetLen)
			if err != nil {
				return "", err
			}
			result[i] = charsetBytes[n.Int64()]
		}

		return string(result), nil
	})

	if err != nil {
		panic("random string generation operation failed: " + err.Error())
	}

	return strInterface.(string)
}

// Sleep provides deterministic sleep functionality
// Note: During replay, this will be a no-op since the time delay is not replayable
func (d *DeterministicRuntime) Sleep(duration time.Duration) {
	seq := d.getNextCallSequence()
	op := NewOperation(
		"time_operation",
		"",
		d.execution.currentPathID,
		map[string]interface{}{
			"operation":     "sleep",
			"duration":      duration.String(),
			"call_sequence": seq,
		},
	)

	_, err := d.execution.ExecuteOperation(context.Background(), op, func() (interface{}, error) {
		time.Sleep(duration)
		return nil, nil
	})

	if err != nil {
		panic("sleep operation failed: " + err.Error())
	}
}
