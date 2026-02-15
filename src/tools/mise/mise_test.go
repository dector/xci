package mise

import (
	"reflect"
	"testing"
)

func TestParseOutdatedJSON(t *testing.T) {
	t.Parallel()

	input := `{"python": {"requested": "3.11", "current": "3.11.0", "latest": "3.11.1"}, "node": {"requested": "20", "current": "20.0.0", "latest": "20.1.0"}}`

	got, err := parseOutdatedJSON(input)
	if err != nil {
		t.Fatalf("parseOutdatedJSON returned error: %v", err)
	}

	want := []OutdatedPackage{
		{Name: "node", Requested: "20", Current: "20.0.0", Latest: "20.1.0"},
		{Name: "python", Requested: "3.11", Current: "3.11.0", Latest: "3.11.1"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected packages\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestParseOutdatedJSONEmpty(t *testing.T) {
	t.Parallel()

	got, err := parseOutdatedJSON("{}")
	if err != nil {
		t.Fatalf("parseOutdatedJSON returned error: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("expected no packages, got %d", len(got))
	}
}

func TestParseOutdatedJSONUpToDateMessage(t *testing.T) {
	t.Parallel()

	got, err := parseOutdatedJSON("mise All tools are up to date")
	if err != nil {
		t.Fatalf("parseOutdatedJSON returned error: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("expected no packages, got %d", len(got))
	}
}

func TestBuildFailureReason(t *testing.T) {
	t.Parallel()

	err := fakeErr("exit status 1")
	output := "line one\nline two\nfinal reason"

	got := buildFailureReason(err, output)
	want := "final reason (exit status 1)"
	if got != want {
		t.Fatalf("unexpected reason\nwant: %q\ngot:  %q", want, got)
	}
}

type fakeErr string

func (e fakeErr) Error() string {
	return string(e)
}
