package riven

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type corpusProfile struct {
	edition            string
	commands, archives int
}

// Editions are selected explicitly: archive contents identify a regression
// fixture, not the version of an interpreter the importer does not execute.
func lookupCorpusProfile(edition string) (corpusProfile, error) {
	switch edition {
	case "", "1.02":
		return corpusProfile{"1.02", 45491, 28}, nil
	case "1.0":
		return corpusProfile{"1.0", 45502, 26}, nil
	default:
		return corpusProfile{}, fmt.Errorf("unsupported RIVEN_TEST_EDITION %q; use 1.0 or 1.02", edition)
	}
}

func fixtureCorpus(t *testing.T) (string, corpusProfile) {
	t.Helper()
	input := os.Getenv("RIVEN_TEST_DATA")
	if input == "" {
		input = os.Getenv("RIVEN_TEST_INPUT") // Retain the original bitmap-test alias.
	}
	if input == "" {
		t.Skip("set RIVEN_TEST_DATA to a verified five-CD directory for corpus checks")
	}
	profile, err := lookupCorpusProfile(os.Getenv("RIVEN_TEST_EDITION"))
	if err != nil {
		t.Fatal(err)
	}
	return input, profile
}

func fixtureInput(t *testing.T) string {
	t.Helper()
	input, _ := fixtureCorpus(t)
	return input
}

func TestCorpusEditionSelection(t *testing.T) {
	for _, edition := range []string{"", "1.0", "1.02"} {
		profile, err := lookupCorpusProfile(edition)
		if err != nil {
			t.Fatal(err)
		}
		expected := edition
		if expected == "" {
			expected = "1.02"
		}
		if profile.edition != expected {
			t.Fatalf("edition %q selected %+v", edition, profile)
		}
	}
	for _, edition := range []string{"1.01", "auto", "1.0.0.0"} {
		if _, err := lookupCorpusProfile(edition); err == nil {
			t.Fatalf("accepted unverified regression edition %q", edition)
		}
	}
}

// These are full archive SHA-256 fingerprints, rather than the short prefix
// hashes used by some game detectors. No game data is stored in the repository.
// The common banks were checked independently in the 1.0 and 1.02 CD sets.
type corpusArchive struct{ name, sha256 string }

var commonCorpusArchives = map[corpusArchive]int{
	{"A_DATA.MHK", "8ac1c23f6fe2c8ea4efd23cb1830f0dffd044288d9aecec717822d7fb639290f"}:   1,
	{"A_SOUNDS.MHK", "91e0b5f41a55da9a0d7307e682c60e5c5d028de61142708826bf8863e65b0932"}: 2,
	{"B2_DATA.MHK", "05445cb34e6b7adebd942ecf3fe38cd16125993b7e690ebd7d767d5a74e455f9"}:  1,
	{"B_DATA.MHK", "8c2a88dac31a0220605d9427782da8d2dd2bc697cc4d91885cd17537650d4745"}:   1,
	{"B_SOUNDS.MHK", "5d1a885ce8b49c897838157a26c842cc2aca900035dbe7fb0fd23e97e83d6a0b"}: 1,
	{"B_SOUNDS.MHK", "caa6f9bae659265154e71ca0e0351cf687fdd2e0957e9191abb319262a7a5e93"}: 1,
	{"G_DATA.MHK", "7c3a276c784bfaea5aa30188548b51bccce9ee3248a28fabb55a91562bff0537"}:   1,
	{"G_SOUNDS.MHK", "31c2d07f197b534c094708a4549696e3c6f1f0eb95dfd92a03e9784baffd8a18"}: 1,
	{"G_SOUNDS.MHK", "4d66d7466cdd3d7ce0cacad751c7c751ac305d84c55b80339eca43b9cc4c0a11"}: 1,
	{"J_DATA1.MHK", "ec2b8cada0f560916cfba08145f08cb7eafa2985a9338c1915edb0dadc4eaa98"}:  1,
	{"J_DATA2.MHK", "e69f8ee7d17c3bf6ee54183f5745a4b86401486854962d5d7e47672e8c85a644"}:  1,
	{"J_SOUNDS.MHK", "cde48f8b405df9afd19c6a5299e486f28d4646e51dcee568542658f30f5fe451"}: 1,
	{"J_SOUNDS.MHK", "f98f5d3fec6f6d1c8eaafceaee342684f4e15afbe7a19b244ce048eb9e9d0b5c"}: 1,
	{"O_DATA.MHK", "fbdeb926142896d44628b149a00c218ef8a7de3c1adeb31cca5def113129bbb5"}:   1,
	{"O_SOUNDS.MHK", "891b0c22a38af07de4c7688fdba26ec6e63e987444343070a44089a0cf43cd6d"}: 1,
	{"O_SOUNDS.MHK", "b2712b86bb284403a8b68f33d883339ce55ed6b100fe75bb1ed16d8da6996ae4"}: 1,
	{"P_DATA.MHK", "b9b80bc8012c9c4248f6b3067d9cd9f40b37ea65eae0b3fe8e7d32888a043eaa"}:   1,
	{"P_SOUNDS.MHK", "047e37af947a8be956c2486d97612ac5f2c48092d7ff24c9ebcc497e23ac67a8"}: 1,
	{"P_SOUNDS.MHK", "36451f88215fd1ec7460ebb673a77ad91ea49f1c2da00b88fe8534f4baa6cd0c"}: 1,
	{"R_DATA.MHK", "ab10561a7026eee98b9741de8237576a90c2243f8f8919ec175175f17774e803"}:   1,
	{"R_SOUNDS.MHK", "4f41f91049c657575fd028409c6bc42b11c4806507a85d2c967d4f405822ed3e"}: 1,
	{"R_SOUNDS.MHK", "eaf80b02b25fd4d82d278e2b4cdb098254bdaf9c853dc24245e08e4430a7a20e"}: 1,
	{"T_DATA.MHK", "725e0342efe159d296eb9cf690eb9dcbf3f30c7e1356df0201bccdd22cf22b05"}:   1,
	{"T_SOUNDS.MHK", "078a9c0856770644af3210a4eef98f66e601629e9b87802d11c91c2c7fb4f163"}: 1,
	{"T_SOUNDS.MHK", "674ef1f0559645478fb2f15d334e61ddad3ce56741d9346de41f12ca316068a4"}: 1,
}

var updatedCorpusArchives = map[corpusArchive]int{
	{"B_DATA1.MHK", "6f6c1c5539b5879e0e43dbc56c8c40010b9979e084ffefe2be1a1f691c0424f2"}: 1,
	{"J_DATA3.MHK", "ca0b4025a1df2f43f3dc7c31c56fc84d94d17cd7006ae3a433867080f2b59a5d"}: 1,
}

func TestSuppliedDiscArchiveFingerprints(t *testing.T) {
	input, profile := fixtureCorpus(t)
	archives, err := DiscoverArchives(input)
	if err != nil {
		t.Fatal(err)
	}
	want := make(map[corpusArchive]int, profile.archives)
	for archive, count := range commonCorpusArchives {
		want[archive] = count
	}
	if profile.edition == "1.02" {
		for archive, count := range updatedCorpusArchives {
			want[archive] = count
		}
	}
	if len(archives) != profile.archives {
		t.Fatalf("edition %s: found %d archives, want %d; choose RIVEN_TEST_EDITION explicitly for the supplied corpus", profile.edition, len(archives), profile.archives)
	}
	for _, archive := range archives {
		source := archive.Source
		file, err := os.Open(source.Path)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.New()
		n, copyErr := io.Copy(hash, io.NewSectionReader(file, source.Offset, source.Size))
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil || n != source.Size {
			t.Fatalf("cannot fingerprint %s: copied %d/%d, read=%v, close=%v", source.Name, n, source.Size, copyErr, closeErr)
		}
		key := corpusArchive{strings.ToUpper(filepath.Base(source.Name)), fmt.Sprintf("%x", hash.Sum(nil))}
		if want[key] == 0 {
			t.Fatalf("edition %s: unexpected or repeated archive %s with SHA-256 %s", profile.edition, source.Name, key.sha256)
		}
		want[key]--
	}
	for archive, count := range want {
		if count != 0 {
			t.Fatalf("edition %s: missing %d copy/copies of %s with SHA-256 %s", profile.edition, count, archive.name, archive.sha256)
		}
	}
	t.Logf("edition %s: all %d complete archive SHA-256 fingerprints match", profile.edition, len(archives))
}
