package runtime

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestDispatchVersion(t *testing.T) {
	var out bytes.Buffer
	if err := Dispatch([]string{"version"}, &out); err != nil {
		t.Fatalf("Dispatch(version) error: %v", err)
	}
	if !strings.Contains(out.String(), "media-core") {
		t.Errorf("version output %q does not contain the build identity", out.String())
	}
}

func TestDispatchMissing(t *testing.T) {
	if err := Dispatch(nil, &bytes.Buffer{}); !errors.Is(err, ErrMissingCommand) {
		t.Errorf("Dispatch(nil) = %v, want ErrMissingCommand", err)
	}
}

func TestDispatchReserved(t *testing.T) {
	for _, command := range []string{"serve", "mcp"} {
		var out bytes.Buffer
		err := Dispatch([]string{command}, &out)
		var reserved ErrReserved
		if !errors.As(err, &reserved) || reserved.Command != command {
			t.Errorf("Dispatch(%s) = %v, want ErrReserved{%s}", command, err, command)
		}
		if out.Len() != 0 {
			t.Errorf("Dispatch(%s) wrote %q to out", command, out.String())
		}
	}
}

func TestDispatchUnsupported(t *testing.T) {
	var out bytes.Buffer
	err := Dispatch([]string{"frobnicate"}, &out)
	var unsupported ErrUnsupported
	if !errors.As(err, &unsupported) || unsupported.Command != "frobnicate" {
		t.Errorf("Dispatch(frobnicate) = %v, want ErrUnsupported", err)
	}
	if out.Len() != 0 {
		t.Errorf("Dispatch(frobnicate) wrote %q to out", out.String())
	}
}
