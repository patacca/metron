package core

import (
	"fmt"
	"slices"
)

// Ingestion of verifier reports.

func (in *ingestion) checkVerifier(b VerifierBody) {
	seen := make(map[string]string)
	for i, v := range b.Verifications {
		p := ptr("body", "verifications", i)
		if !slices.Contains(in.pk.Subjects, v.Finding) {
			in.sem.add(p+"/finding", "finding %q is not a claimed fix of packet %s: verify only the packet's subjects", v.Finding, in.pk.Packet)
		} else if prev, dup := seen[v.Finding]; dup {
			in.sem.add(p+"/finding", "finding %s is already verified at %s: verify each fix once", v.Finding, prev)
		} else {
			seen[v.Finding] = p
		}
		if v.Result != VerificationConfirmed && v.Result != VerificationRejected {
			in.sem.add(p+"/result", "%q is not %q or %q", v.Result, VerificationConfirmed, VerificationRejected)
		}
		if blank(v.Evidence) {
			in.sem.add(p+"/evidence", "the evidence is blank: say what the result rests on")
		}
	}
	for _, id := range in.pk.Subjects {
		if _, ok := seen[id]; !ok {
			in.sem.add("/body/verifications", "the fix of %s has no verification: confirm or reject every claimed fix", id)
		}
	}
	keys := make(map[string]string)
	for i, rg := range b.Regressions {
		in.checkFinding(ptr("body", "regressions", i), rg, keys, true)
	}
}

func (in *ingestion) verifierTransition(b VerifierBody) {
	for i, v := range b.Verifications {
		if st := in.find(v.Finding).Status; st != StatusClaimed {
			in.tr.add(ptr("body", "verifications", i, "finding"), "finding %s is %s: only a claimed fix can be verified", v.Finding, st)
		}
	}
}

func (in *ingestion) verifierEvents(b VerifierBody) {
	pk := in.pk
	confirmed := 0
	for _, v := range b.Verifications {
		if v.Result == VerificationConfirmed {
			confirmed++
			in.add(EventFindingFixConfirmed, FindingFixConfirmed{ID: v.Finding, Packet: pk.Packet, Evidence: v.Evidence})
		} else {
			in.add(EventFindingFixRejected, FindingFixRejected{ID: v.Finding, Packet: pk.Packet, Evidence: v.Evidence,
				RootCauseRequired: in.find(v.Finding).Recurrences+1 >= in.th.RootCauseAfter})
		}
	}
	var blocking []string
	for _, rg := range b.Regressions {
		id := in.open(RoleVerifier, rg)
		if rg.Tier.Blocking() {
			blocking = append(blocking, id)
		}
		if rg.Contradicts != "" {
			in.raise(EscalationContradiction, fmt.Sprintf("%s declares that it reverses the resolution of %s", id, rg.Contradicts), []string{id}, nil, nil)
		}
	}
	if len(blocking) > 0 && len(blocking) >= confirmed {
		in.raise(EscalationRegressionSpiral, fmt.Sprintf("verification %s found %s and confirmed %s", pk.Packet,
			count(len(blocking), "blocking regression", "blocking regressions"), count(confirmed, "fix", "fixes")), blocking, nil, nil)
	}
}
