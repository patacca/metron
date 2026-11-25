package core

import (
	"fmt"
	"slices"
	"strings"
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
		address(p, fx.Finding, slices.Contains(in.pk.Subjects, fx.Finding), "a subject")
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
	// An open subject is fixed or disputed. One with a claimed fix is left
	// alone or claimed again; which of the two depends on the candidate, and
	// is decided with the state.
	for _, id := range in.pk.Subjects {
		if _, ok := addressed[id]; !ok && in.find(id).Status != StatusClaimed {
			in.sem.add("/body", "blocking finding %s is not addressed: fix or dispute every open subject of the packet", id)
		}
	}
}

// unaccounted returns the commits of the correction interval that change the
// candidate and that no fix lists, in this report or in an earlier accepted
// one.
func (in *ingestion) unaccounted(b ImplementerBody) []Commit {
	var out []Commit
	for _, c := range in.f.Interval {
		listed := slices.Contains(in.s.accounted, c.Hash) ||
			slices.ContainsFunc(b.Fixes, func(fx Fix) bool { return slices.Contains(fx.Commits, c.Hash) })
		if c.Substantive && !listed {
			out = append(out, c)
		}
	}
	return out
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
		// A subject is open, or claimed on a candidate that has changed
		// since, in which case its fix is claimed again.
		if x.Status != StatusOpen && x.Status != StatusClaimed {
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
		case x.Status == StatusClaimed:
			in.tr.add(ptr("body", "disputes", i, "finding"), "finding %s already has a claimed fix: claim it again with every commit that belongs to it, in fixes", x.ID)
		case x.Status != StatusOpen:
			in.tr.add(ptr("body", "disputes", i, "finding"), "finding %s is %s: only an open finding can be disputed", x.ID, x.Status)
		}
	}
	// A claim is about one candidate. On another one, every claimed fix is
	// claimed again, so that each goes to a verifier as a claim made on the
	// candidate it is verified on, with the commits it had before.
	for _, id := range in.pk.Subjects {
		x := in.find(id)
		if x.Status != StatusClaimed || x.Fix == nil {
			continue
		}
		i := slices.IndexFunc(b.Fixes, func(fx Fix) bool { return fx.Finding == id })
		if i < 0 {
			if in.f.Candidate.Digest != in.s.claimedOn {
				in.tr.add("/body/fixes", "the fix of %s was claimed on candidate %s and the candidate is now %s: claim it again in this report, with every commit that belongs to it", id, in.s.claimedOn, in.f.Candidate.Digest)
			}
			continue
		}
		for _, c := range x.Fix.Commits {
			still := slices.ContainsFunc(in.f.Interval, func(ic Commit) bool { return ic.Hash == c })
			if still && !slices.Contains(b.Fixes[i].Commits, c) {
				in.tr.add(ptr("body", "fixes", i, "commits"), "commit %s belonged to the fix of %s when it was claimed before and is still part of the candidate: a fix claimed again lists every commit that belongs to it", c, id)
			}
		}
	}
	// Every commit that changed the candidate since the reviewed one belongs
	// to a fix. The rule holds while the reviewed commit still exists.
	if missing := in.unaccounted(b); len(missing) > 0 {
		const show = 5
		var names []string
		for _, c := range missing[:min(show, len(missing))] {
			names = append(names, c.Hash)
		}
		more := ""
		if len(missing) > show {
			more = fmt.Sprintf(" and %d more", len(missing)-show)
		}
		in.tr.add("/body/fixes", "%s %s%s %s the candidate since the reviewed commit %s, and no fix lists %s: every such commit belongs to a fix, so list each under the finding it serves, or take what serves no finding out of the candidate's history",
			pick(len(missing), "commit", "commits"), strings.Join(names, ", "), more, pick(len(missing), "changes", "change"),
			in.pk.Base.Commit, pick(len(missing), "it", "them"))
	}
}

// changed returns the paths a fix's commits change, sorted and each once.
func (in *ingestion) changed(fx Fix) []string {
	paths := []string{}
	for _, c := range in.f.Commits {
		if !slices.Contains(fx.Commits, c.Hash) {
			continue
		}
		for _, path := range c.Paths {
			if !slices.Contains(paths, path) {
				paths = append(paths, path)
			}
		}
	}
	slices.Sort(paths)
	return paths
}

func (in *ingestion) implementerEvents(b ImplementerBody) {
	pk := in.pk
	changed := make([][]string, len(b.Fixes))
	for i, fx := range b.Fixes {
		changed[i] = in.changed(fx)
		in.add(EventFindingFixClaimed, FindingFixClaimed{ID: fx.Finding, Round: pk.Round, Packet: pk.Packet, Candidate: in.f.Candidate.Digest,
			Fix: fx, Paths: changed[i]})
	}
	disputes := slices.Clone(b.Disputes)
	slices.SortStableFunc(disputes, func(a, b Dispute) int { return compareIDs(a.Finding, b.Finding) })
	for _, d := range disputes {
		in.raise(EscalationDispute, fmt.Sprintf("the implementer disputes %s: %s", d.Finding, d.Rationale), []string{d.Finding}, nil, nil)
	}
	// Churn is counted on the paths the fixes change, which are facts, not
	// on the component names a report gives, which are not comparable. One
	// escalation names every path that reaches the threshold with this
	// report.
	var paths, findings []string
	for i, fx := range b.Fixes {
		for _, path := range changed[i] {
			rounds, done := in.s.churnRounds(path)
			n := len(rounds)
			if !slices.Contains(rounds, pk.Round) {
				n++
			}
			if done || n < ComponentChurnRounds {
				continue
			}
			if !slices.Contains(paths, path) {
				paths = append(paths, path)
			}
			if !slices.Contains(findings, fx.Finding) {
				findings = append(findings, fx.Finding)
			}
		}
	}
	if len(paths) > 0 {
		slices.Sort(paths)
		slices.SortFunc(findings, compareIDs)
		in.raise(EscalationComponentChurn, fmt.Sprintf("fixes changed %s in %d distinct rounds of this campaign: %s",
			pick(len(paths), "the same path", "the same paths"), ComponentChurnRounds, strings.Join(paths, ", ")), findings, paths, nil)
	}
}
