package mocking_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/supportapplibs/go-lib/mocking"
)

func TestProduceSampleError_NoArgs(t *testing.T) {
	err := mocking.ProduceSampleError()

	assert.Error(t, err)
	assert.Equal(t, mocking.ErrExpectedNetwork, err.Error())
}

func TestProduceSampleError_SingleError(t *testing.T) {
	inputErr := errors.New("single error")
	err := mocking.ProduceSampleError(inputErr)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "single error")
}

func TestProduceSampleError_MultipleErrors(t *testing.T) {
	err1 := errors.New("error one")
	err2 := errors.New("error two")
	err3 := errors.New("error three")

	err := mocking.ProduceSampleError(err1, err2, err3)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error one")
	assert.Contains(t, err.Error(), "error two")
	assert.Contains(t, err.Error(), "error three")
}

func TestProduceSampleError_WithNewlines(t *testing.T) {
	err1 := errors.New("first")
	err2 := errors.New("second")

	err := mocking.ProduceSampleError(err1, err2)

	lines := strings.Split(err.Error(), "\n")
	assert.True(t, len(lines) >= 2)
}

func TestProduceSampleError_InfraError(t *testing.T) {
	infraErr := errors.New(mocking.ErrExpectedInfra)
	err := mocking.ProduceSampleError(infraErr)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), mocking.ErrExpectedInfra)
}

func TestProduceSampleError_NetworkError(t *testing.T) {
	networkErr := errors.New(mocking.ErrExpectedNetwork)
	err := mocking.ProduceSampleError(networkErr)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), mocking.ErrExpectedNetwork)
}

func TestProduceSampleError_EmptySlice(t *testing.T) {
	err := mocking.ProduceSampleError([]error{}...)

	assert.Error(t, err)
	assert.Equal(t, mocking.ErrExpectedNetwork, err.Error())
}

func TestProduceSampleError_MixedErrors(t *testing.T) {
	err1 := errors.New("database error")
	err2 := errors.New("network timeout")
	err3 := errors.New("invalid input")

	err := mocking.ProduceSampleError(err1, err2, err3)

	assert.Error(t, err)
	errStr := err.Error()
	assert.Contains(t, errStr, "database error")
	assert.Contains(t, errStr, "network timeout")
	assert.Contains(t, errStr, "invalid input")
}

func TestConstants(t *testing.T) {
	assert.Equal(t, "called error expected network", mocking.ErrExpectedNetwork)
	assert.Equal(t, "called error expected infra", mocking.ErrExpectedInfra)
}
