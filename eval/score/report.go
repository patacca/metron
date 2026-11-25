package main

import (
	"fmt"
	"strings"
)

// markdown renders the results. Everything it iterates over is a slice in a
// fixed order, so the same results always give the same bytes.
func markdown(res results) string {
	var b strings.Builder
	p := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	p("# Evaluation results\n\n")
	p("%d %s. Unsafe GREEN: %d; shipped unsafe at the bounded loop's cap: %d.\n", res.Trials, plural(res.Trials, "trial", "trials"), res.UnsafeGreen, res.UnsafeAtCap)
	p("An unsafe GREEN is an approval while a blocker plant's oracle tests do not all pass on the final commit.\n")

	for _, c := range res.Cases {
		p("\n## Case %s\n\n", c.Case)
		if c.Base != "" {
			p("Variant of %s. ", c.Base)
		}
		p("Blocker plants: %s. Decoys: %s. Expected end: %s.\n", list(c.BlockerPlants), list(c.Decoys), c.ExpectedEnd)

		p("\n### Outcomes\n\n")
		p("| Arm | Trials | End states | Approved | Unsafe GREEN | Correct GREEN | Shipped at cap (unsafe) | Stopped for governance: justified / unnecessary | Governance kinds |\n")
		p("| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
		for _, a := range c.Arms {
			s := a.Summary
			p("| %s | %d | %s | %d | %d | %d | %s | %s | %s |\n", a.Arm, s.Trials, countList(s.EndStates), s.Approved, s.UnsafeGreen, s.CorrectGreen,
				onlyFor(a.Arm == armBounded, fmt.Sprintf("%d (%d)", s.ShippedAtCap, s.UnsafeAtCap)),
				onlyFor(a.Arm == armMetron, fmt.Sprintf("%d / %d", s.GovernanceJustified, s.GovernanceUnnecessary)),
				onlyFor(a.Arm == armMetron, countList(s.GovernanceKinds)))
		}

		p("\n### Findings\n\n")
		p("Blocker recall counts plants over all trials. A false blocking finding is a blocking one that matches no blocker plant.\n\n")
		p("| Arm | Recall, round 1 | Recall, end | Found as blocker, end | Late discoveries | False blocking findings | matching a decoy | withdrawn or reclassified |\n")
		p("| --- | --- | --- | --- | --- | --- | --- | --- |\n")
		for _, a := range c.Arms {
			s := a.Summary
			p("| %s | %s | %s | %s | %d | %d | %d | %s |\n", a.Arm, ratio(s.FoundRound1, s.BlockerPlants), ratio(s.FoundEnd, s.BlockerPlants),
				ratio(s.FoundAsBlocker, s.BlockerPlants), s.LateDiscoveries, s.FalseBlocking, s.FalseBlockingDecoy,
				onlyFor(a.Arm == armMetron, fmt.Sprint(s.FalseBlockingRetracted)))
		}

		p("\n### Process\n\n")
		p("| Arm | Rounds | omp runs | Checks | Packets | Reports accepted | Check recordings | Recurring findings | Verifier findings | Excessive fixes |\n")
		p("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
		for _, a := range c.Arms {
			s := a.Summary
			m := a.Arm == armMetron
			p("| %s | %d | %d | %s | %s | %s | %s | %s | %s | %s |\n", a.Arm, s.Rounds, s.OmpRuns, onlyFor(m, fmt.Sprint(s.Checks)),
				onlyFor(m, fmt.Sprint(s.Packets)), onlyFor(m, fmt.Sprint(s.Reports)), onlyFor(m, fmt.Sprint(s.EvidenceBatches)),
				onlyFor(m, fmt.Sprint(s.RecurringFindings)), onlyFor(m, fmt.Sprint(s.VerifierFindings)), onlyFor(m, fmt.Sprint(s.ExcessiveFixes)))
		}

		p("\n### Final commit\n\n")
		p("| Arm | Fixture tests failing | Decoy oracle tests failing | Lines changed | Files changed outside the plants' paths |\n")
		p("| --- | --- | --- | --- | --- |\n")
		for _, a := range c.Arms {
			s := a.Summary
			p("| %s | %d | %d | %d | %d |\n", a.Arm, s.FixtureFailures, s.DecoyOracleFailures, s.LinesChanged, s.FilesOutsidePlants)
		}

		p("\n### Time and tokens\n\n")
		p("| Arm | Seconds | per trial | Input tokens | per trial | Output tokens | per trial |\n")
		p("| --- | --- | --- | --- | --- | --- | --- |\n")
		for _, a := range c.Arms {
			s := a.Summary
			p("| %s | %d | %s | %d | %s | %d | %s |\n", a.Arm, s.Seconds, mean(s.Seconds, s.Trials),
				s.InputTokens, mean(s.InputTokens, s.Trials), s.OutputTokens, mean(s.OutputTokens, s.Trials))
		}

		p("\n### Trials\n")
		for _, a := range c.Arms {
			for _, t := range a.Trials {
				trialMarkdown(&b, a.Arm, t)
			}
		}
	}
	return b.String()
}

func trialMarkdown(b *strings.Builder, arm string, t trialScore) {
	p := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	p("\n#### %s, trial %s\n\n", arm, t.Trial)
	verdict := "not a GREEN"
	switch {
	case t.UnsafeGreen:
		verdict = "**unsafe GREEN**"
	case t.CorrectGreen:
		verdict = "correct GREEN"
	case t.EndState == endApproved:
		verdict = "GREEN with the fixture tests failing"
	case t.UnsafeAtCap:
		verdict = "**shipped unsafe at the cap**"
	case t.EndState == endCap:
		verdict = "shipped at the cap"
	case t.Governance != nil && t.Governance.Justified:
		verdict = "stopped for governance, as the case expects"
	case t.Governance != nil:
		verdict = "stopped for governance, which the case does not expect"
	}
	p("End state `%s`: %s. %d s, %d omp %s", t.EndState, verdict, t.Seconds, t.OmpRuns, plural(t.OmpRuns, "run", "runs"))
	if arm == armMetron {
		p(", %d %s", t.Checks, plural(t.Checks, "check", "checks"))
	}
	p(", %d %s, %d input and %d output tokens.\n\n", t.Rounds, plural(t.Rounds, "round", "rounds"), t.InputTokens, t.OutputTokens)

	p("| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |\n")
	p("| --- | --- | --- | --- | --- | --- |\n")
	for _, r := range t.Plants {
		p("| %s | %s | %s | %s | %s | %s |\n", r.ID, passFail(r.OraclePass), round(r.FirstRound), round(r.FirstRoundAsBlocker),
			list(r.Findings), onlyFor(arm == armMetron, fmt.Sprint(r.FixClaims)))
	}
	p("\n")
	if len(t.FalseBlocking) == 0 {
		p("- False blocking findings: none.\n")
	} else {
		p("- False blocking findings:\n")
		for _, f := range t.FalseBlocking {
			p("  - %s (round %d) %q", f.Finding, f.Round, f.Title)
			if len(f.Decoys) > 0 {
				p(", matches %s", list(f.Decoys))
			}
			if f.Retracted != "" {
				p(", %s", f.Retracted)
			}
			p("\n")
		}
	}
	if m := t.Metron; m != nil {
		p("- Ledger: %d packets, %d reports accepted, %d check recordings, %d verifier findings, %d excessive fixes.\n",
			m.Packets, m.Reports, m.EvidenceBatches, m.VerifierFindings, m.ExcessiveFixes)
		if len(m.Recurring) > 0 {
			var parts []string
			for _, r := range m.Recurring {
				parts = append(parts, fmt.Sprintf("%s (%d rejected, %d reopened)", r.Finding, r.FixRejected, r.Reopened))
			}
			p("- Recurring findings: %s.\n", strings.Join(parts, ", "))
		}
	}
	p("- Fixture tests: %s. Decoy oracle tests failing: %s.\n", t.FixtureTests, list(t.DecoyOracleFailures))
	p("- %d lines changed; files changed outside the plants' paths: %s.\n", t.LinesChanged, list(t.FilesOutsidePlants))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func list(s []string) string {
	if len(s) == 0 {
		return "none"
	}
	return strings.Join(s, ", ")
}

func countList(cs []count) string {
	if len(cs) == 0 {
		return "none"
	}
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = fmt.Sprintf("%s %d", c.Name, c.N)
	}
	return strings.Join(parts, ", ")
}

func ratio(n, of int) string {
	if of == 0 {
		return "0/0"
	}
	return fmt.Sprintf("%d/%d (%d%%)", n, of, n*100/of)
}

func mean(total, n int) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f", float64(total)/float64(n))
}

func round(r int) string {
	if r == 0 {
		return "never"
	}
	return fmt.Sprint(r)
}

func passFail(ok bool) string {
	if ok {
		return "pass"
	}
	return "**fail**"
}

// onlyFor shows v for the arms a measure applies to, and a dash for others.
func onlyFor(applies bool, v string) string {
	if applies {
		return v
	}
	return "-"
}
