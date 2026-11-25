package core

import (
	"fmt"
	"slices"
)

// Ingestion of implementer reports.

func (in *ingestion) checkImplementer(b ImplementerBody) {
	addressed := make(map[string]string)
	address := func(p, id string, allowed bool, what string) {
		switch prev, dup := addressed[id]; {
		case !allowed:
			in.sem.add(p+"/finding", "finding %q is not %s of packet %s", id, what, in.pk.Packet)
		case dup:
			in.sem.add(p+"/finding", "finding %s is already addressed at %s: address each finding once", id, prev)
		default:
			addressed[id] = p
		}
	}
	for i, fx := range b.Fixes {
		p := ptr("body", "fixes", i)
		allowed := slices.Contains(in.pk.Subjects, fx.Finding) || slices.Contains(in.pk.Backlog, fx.Finding)
		address(p, fx.Finding, allowed, "a subject or a backlog item")
		if len(fx.Commits) == 0 {
			in.sem.add(p+"/commits", "no commit is listed: list the full hashes of the commits that contain the fix")
		}
		for j, h := range fx.Commits {
			if !validCommit(h) {
				in.sem.add(fmt.Sprintf("%s/commits/%d", p, j), "%q is not a full commit hash", h)
			} else if slices.Index(fx.Commits, h) < j {
				in.sem.add(fmt.Sprintf("%s/commits/%d", p, j), "commit %s is listed twice", h)
			}
		}
		if blank(fx.Summary) {
			in.sem.add(p+"/summary", "the summary is blank: say what the fix changes")
		}
		if len(fx.Components) == 0 {
			in.sem.add(p+"/components", "no component is named: name at least one file, module or subsystem the fix touched")
		}
		statements(&in.sem, p+"/components", fx.Components, "component")
		if slices.Contains(in.pk.RootCauseRequired, fx.Finding) && blank(fx.RootCause) {
			in.sem.add(p+"/root_cause", "finding %s is flagged root-cause-required: include a root-cause analysis", fx.Finding)
		}
	}
	for i, d := range b.Disputes {
		p := ptr("body", "disputes", i)
		address(p, d.Finding, slices.Contains(in.pk.Subjects, d.Finding), "a blocking subject that can be disputed")
		if blank(d.Rationale) {
			in.sem.add(p+"/rationale", "the rationale is blank: say why the finding should not block")
		}
	}
	for _, id := range in.pk.Subjects {
		if _, ok := addressed[id]; !ok {
			in.sem.add("/body", "blocking finding %s is not addressed: fix or dispute every subject of the packet", id)
		}
	}
}

func (in *ingestion) implementerTransition(b ImplementerBody) {
	if !in.f.Clean {
		in.tr.add("", "the working tree has changes outside .metron/: commit them, then submit the report")
	}
	commits := make(map[string]Commit)
	for _, c := range in.f.Commits {
		commits[c.Hash] = c
	}
	for i, fx := range b.Fixes {
		for j, h := range fx.Commits {
			p := ptr("body", "fixes", i, "commits", j)
			switch c, ok := commits[h]; {
			case !ok:
				in.tr.add(p, "commit %s could not be examined in the repository, so it cannot be accepted: list full hashes of commits that exist", h)
			case !c.Reachable:
				in.tr.add(p, "commit %s is not part of the current candidate's history: list commits that HEAD contains", h)
			case c.InBase:
				in.tr.add(p, "commit %s is already contained in the round's candidate: list only commits made since then", h)
			case !c.Substantive:
				in.tr.add(p, "commit %s changes nothing outside .metron/ and so fixes nothing: list the commits that change the candidate", h)
			}
		}
		x := in.find(fx.Finding)
		if x.Status != StatusOpen {
			in.tr.add(ptr("body", "fixes", i, "finding"), "finding %s is %s: only an open finding can be fixed", x.ID, x.Status)
			continue
		}
		candidate := in.f.Candidate.Digest
		if len(x.RecordedOn()) == 0 {
			in.tr.add(ptr("body", "fixes", i), "finding %s has no recorded candidate: the ledger is inconsistent", x.ID)
		} else if by, ok := x.recordedBy(candidate); ok {
			in.tr.add(ptr("body", "fixes", i), "the candidate is %s, on which %s was recorded as present by %s in packet %s: a fix must produce a candidate the finding was never recorded on; commit a change that fixes it, then submit the report",
				candidate, x.ID, by.Event, by.Packet)
		}
	}
	for i, d := range b.Disputes {
		x := in.find(d.Finding)
		switch {
		case x.Disputed:
			in.tr.add(ptr("body", "disputes", i, "finding"), "finding %s was already disputed: after adjudication it must be fixed", x.ID)
		case x.Status != StatusOpen:
			in.tr.add(ptr("body", "disputes", i, "finding"), "finding %s is %s: only an open finding can be disputed", x.ID, x.Status)
		}
	}
}

func (in *ingestion) implementerEvents(b ImplementerBody) {
	pk := in.pk
	for _, fx := range b.Fixes {
		in.add(EventFindingFixClaimed, FindingFixClaimed{ID: fx.Finding, Round: pk.Round, Packet: pk.Packet, Candidate: in.f.Candidate.Digest, Fix: fx})
	}
	disputes := slices.Clone(b.Disputes)
	slices.SortStableFunc(disputes, func(a, b Dispute) int { return compareIDs(a.Finding, b.Finding) })
	for _, d := range disputes {
		in.raise(EscalationDispute, fmt.Sprintf("the implementer disputes %s: %s", d.Finding, d.Rationale), []string{d.Finding}, nil, nil)
	}
	var components []string
	for _, fx := range b.Fixes {
		for _, c := range fx.Components {
			if !slices.Contains(components, c) {
				components = append(components, c)
			}
		}
	}
	slices.Sort(components)
	for _, c := range components {
		rounds, done := in.s.churnRounds(c)
		n := len(rounds)
		if !slices.Contains(rounds, pk.Round) {
			n++
		}
		if done || n < ComponentChurnRounds {
			continue
		}
		var findings []string
		for _, fx := range b.Fixes {
			if slices.Contains(fx.Components, c) && !slices.Contains(findings, fx.Finding) {
				findings = append(findings, fx.Finding)
			}
		}
		slices.SortFunc(findings, compareIDs)
		in.raise(EscalationComponentChurn, fmt.Sprintf("component %q was named in fixes in %d distinct rounds of stage %s", c, n, in.c.Stage), findings, []string{c}, nil)
	}
}
