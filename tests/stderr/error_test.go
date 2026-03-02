package stderr_test

import (
	"testing"

	"github.com/supportapplibs/go-lib/stderr"
)

func TestErr(t *testing.T) {
	err := stderr.Err("77", "oops", 200)

	// assert the code
	if stderr.GetCode(err) != "99" {
		t.Errorf("expect %s, got %s", "77", stderr.GetCode(err))
	}
	// assert the message
	if stderr.GetMsg(err) != "oops" {
		t.Errorf("expect %s, got %s", "oops", stderr.GetMsg(err))
	}
	// assert the http code
	if stderr.GetHttpCode(err) != 200 {
		t.Errorf("expect %d, got %d", 200, stderr.GetHttpCode(err))
	}
}

func TestErr_Error(t *testing.T) {
	err := stderr.Err("77", "oops", 200)
	if err.Error() != "99 oops" { // -> Map to ErrRuntime, for code except stdCode
		t.Errorf("expect %s, got %s", "77 oops", err.Error())
	}
}

func TestErr_IsErrNotFound(t *testing.T) {
	err := stderr.ErrDataNotFound()
	if !stderr.IsErrNotFound(err) {
		t.Errorf("expect %v, got %v", true, stderr.IsErrNotFound(err))
	}

	err = stderr.ErrThirdParty()
	if stderr.IsErrNotFound(err) {
		t.Errorf("expect %v, got %v", false, stderr.IsErrNotFound(err))
	}
}
