package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/patacca/metron/internal/core"
)

func TestPacketFormats(t *testing.T) {
	r := pinned(t)
	r.must("check")
	first := r.must("packet")
	contains(t, "packet note", first.err, "issued packet P-0001: review for the reviewer, round 1")
	again := r.must("packet")
	contains(t, "packet note", again.err, "packet P-0001 is open and still valid; returning it again")
	if first.out != again.out {
		t.Fatal("packet returned different bytes the second time")
	}
	stored := string(r.read(".metron/state/packets/P-0001.json"))
	if strings.Contains(stored, "\n") || !strings.HasPrefix(first.out, "{\n  \"action\": \"review\"") {
		t.Fatalf("the stored packet is not canonical, or the output not indented:\n%s", first.out[:80])
	}

	prompt := r.must("packet", "--format", "prompt").out
	if prompt != r.must("packet", "--format", "prompt").out {
		t.Fatal("the prompt differs the second time")
	}
	contains(t, "prompt", prompt, "# Packet P-0001", "metron ingest <file>", "# Report skeleton")

	var sk core.Report
	unmarshal(t, r.must("packet", "--format", "skeleton").out, &sk)
	if sk.Schema != core.SchemaReport || sk.Packet != "P-0001" || sk.Role != core.RoleReviewer {
		t.Fatalf("skeleton %+v", sk)
	}

	out := filepath.Join(t.TempDir(), "packet.json")
	res := r.must("packet", "-o", out)
	contains(t, "packet note", res.err, "wrote the json to "+out)
	if data, err := os.ReadFile(out); err != nil || string(data) != first.out || res.out != "" {
		t.Fatalf("packet -o wrote %d bytes (%v) and printed %q", len(data), err, res.out)
	}
	res = r.must("packet", "--format", "prompt", "-o", "prompt.md")
	contains(t, "packet warning", res.err, "warning: prompt.md is in the work tree and makes it dirty")
	r.expectNext(core.StepCommitChanges)
}

func TestPacketFileDamaged(t *testing.T) {
	for _, tc := range []struct {
		name   string
		damage func(r *repo, path string)
		want   string
	}{
		{"deleted", func(r *repo, path string) {
			if err := os.Remove(r.path(path)); err != nil {
				r.t.Fatal(err)
			}
		}, "its stored file packets/P-0001.json is missing"},
		{"altered", func(r *repo, path string) {
			r.write(path, strings.Replace(string(r.read(path)), `"round":1`, `"round":7`, 1))
		}, "its stored file packets/P-0001.json does not match the digest recorded when it was issued"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := pinned(t)
			r.must("check")
			_, sk := r.packet()
			tc.damage(r, ".metron/state/packets/P-0001.json")

			contains(t, "next", r.must("next").out, "Next step:    review",
				"Note:         packet P-0001 (review) is open but no longer valid ("+tc.want+"); metron packet voids it")
			var st statusDoc
			unmarshal(t, r.must("status", "--json").out, &st)
			if st.OpenPacket == nil || st.OpenPacket.Valid || !strings.Contains(st.OpenPacket.Invalid, tc.want) ||
				len(st.StoredFiles) != 1 || !strings.Contains(st.StoredFiles[0].Problem, tc.want) {
				t.Fatalf("status open packet %+v, stored files %+v", st.OpenPacket, st.StoredFiles)
			}
			contains(t, "fsck", r.expect(exitNegative, "fsck").out, tc.want, "run metron packet to void it")
			// The open packet's file is not an integrity failure: the packet is
			// ignored until metron packet voids it.
			if out := r.expect(exitNegative, "gate").out; strings.Contains(out, "P-0001") {
				t.Fatalf("the gate counts the open packet's file:\n%s", out)
			}
			res := r.ingest(exitNegative, review(t, sk, "rev-1", nil))
			contains(t, "ingest error", res.err, "the stored file of packet P-0001 is unusable", "run metron packet again")

			p, _ := r.packet()
			if p.ID != "P-0002" || p.Round != 2 {
				t.Fatalf("reissued packet %s round %d", p.ID, p.Round)
			}
			contains(t, "log", r.must("log").out, "packet.voided          P-0001: "+tc.want)
			r.must("fsck")
			if a := r.expectNext(core.StepSubmitReport); a.Packet != "P-0002" {
				t.Fatalf("next %+v", a)
			}
		})
	}
}
