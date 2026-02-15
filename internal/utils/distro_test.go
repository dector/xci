package utils

import "testing"

func TestDetectDistroFamilyFromOSReleaseFedoraLikeByID(t *testing.T) {
	t.Parallel()

	input := "ID=Fedora\n"
	got := detectDistroFamilyFromOSRelease(input)
	if got != DistroFamilyFedoraLike {
		t.Fatalf("unexpected family\nwant: %q\ngot:  %q", DistroFamilyFedoraLike, got)
	}
}

func TestDetectDistroFamilyFromOSReleaseFedoraLikeByIDLike(t *testing.T) {
	t.Parallel()

	input := "ID=custom\nID_LIKE=\"debian RHEL\" # comment\n"
	got := detectDistroFamilyFromOSRelease(input)
	if got != DistroFamilyFedoraLike {
		t.Fatalf("unexpected family\nwant: %q\ngot:  %q", DistroFamilyFedoraLike, got)
	}
}

func TestDetectDistroFamilyFromOSReleaseNonFedoraLike(t *testing.T) {
	t.Parallel()

	input := "ID=ubuntu\nID_LIKE=debian\n"
	got := detectDistroFamilyFromOSRelease(input)
	if got != DistroFamilyNonFedoraLike {
		t.Fatalf("unexpected family\nwant: %q\ngot:  %q", DistroFamilyNonFedoraLike, got)
	}
}

func TestDetectDistroFamilyFromOSReleaseUnknownMalformed(t *testing.T) {
	t.Parallel()

	input := "# only comments\nNOT_A_KV_PAIR\n"
	got := detectDistroFamilyFromOSRelease(input)
	if got != DistroFamilyUnknown {
		t.Fatalf("unexpected family\nwant: %q\ngot:  %q", DistroFamilyUnknown, got)
	}
}
