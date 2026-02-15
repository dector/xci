package flatpak

import (
	"reflect"
	"testing"
)

func TestParseOutdatedRefs(t *testing.T) {
	t.Parallel()

	input := "Ref\norg.freedesktop.Platform/x86_64/24.08\norg.gnome.Calculator/x86_64/stable\n"

	got, err := parseOutdatedRefs(input)
	if err != nil {
		t.Fatalf("parseOutdatedRefs returned error: %v", err)
	}

	want := []OutdatedPackage{
		{Ref: "org.freedesktop.Platform/x86_64/24.08"},
		{Ref: "org.gnome.Calculator/x86_64/stable"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected packages\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestParseOutdatedRefsEmpty(t *testing.T) {
	t.Parallel()

	got, err := parseOutdatedRefs("")
	if err != nil {
		t.Fatalf("parseOutdatedRefs returned error: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("expected no packages, got %d", len(got))
	}
}

func TestParseOutdatedRefsDeduplicatesAndSorts(t *testing.T) {
	t.Parallel()

	input := "org.z.App/x86_64/stable\norg.a.App/x86_64/stable\norg.z.App/x86_64/stable\n"

	got, err := parseOutdatedRefs(input)
	if err != nil {
		t.Fatalf("parseOutdatedRefs returned error: %v", err)
	}

	want := []OutdatedPackage{
		{Ref: "org.a.App/x86_64/stable"},
		{Ref: "org.z.App/x86_64/stable"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected packages\nwant: %#v\ngot:  %#v", want, got)
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
