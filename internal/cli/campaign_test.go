package cli

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/patacca/metron/internal/core"
	"github.com/patacca/metron/internal/store"
)

// End-to-end campaigns that do not converge: through the commands, the
// schemas, the store and git, they reach governance within the bounds the
// contract sets, stop there, and never approve anything on the way.

// TestCampaignSuspendedAndResumed drives a campaign through every kind of
// failed step, a RED round, a failing check and a rejected fix, to the
// convergence analysis, the process audit and the suspension its discharge
// raises; checks that nothing moves while the campaign is suspended; and
// finishes it cooperatively once governance resumes it. The transcript is a
// golden file, so that any change to the orchestrator contract shows up as a
// diff.
func TestCampaignSuspendedAndResumed(t *testing.T) {
	r := campaign(t, campaignContract(2, 2, 3, 20))
	tr := &transcript{r: r}
	tr.metron(exitOK, core.StepReview, core.RoleReviewer, "check")
	_, sk := tr.packet(core.RoleReviewer)
	first := red(t, sk, "rev-1", misspelled())
	tr.ingest("review: RED", exitOK, first, core.StepFix, core.RoleImplementer)

	// The fix misspells the greeting again and breaks the check.
	_, sk = tr.packet(core.RoleImplementer)
	r.write("greeting.txt", "hullo\n")
	r.write("broken", "yes\n")
	c1 := r.commitOnly("misspell the greeting differently", "greeting.txt", "broken")
	tr.git("commit c1, hullo and broken", core.StepSubmitReport, core.RoleImplementer)
	tr.ingest("fix: claim c1", exitOK, claim(t, sk, "impl-1", "", c1), core.StepRunChecks, core.RoleImplementer)

	// The failing check is the second failed step: metron check raises the
	// convergence analysis.
	res, a := tr.metron(exitNegative, core.StepAudit, core.RoleAuditor, "check", "--json")
	var cd checkDoc
	unmarshal(t, res.out, &cd)
	if cd.Passed || !cd.Recorded || !slices.Equal(cd.EscalationsRaised, []string{"E-0001"}) || !slices.Equal(a.Escalations, []string{"E-0001"}) {
		t.Fatalf("check document %+v, next %+v", cd, a)
	}
	ap, sk := tr.packet(core.RoleAuditor)
	if ap.Round != 1 || len(ap.Escalations) != 1 || ap.Escalations[0].Kind != core.EscalationNonConvergence {
		t.Fatalf("audit packet round %d, escalations %+v", ap.Round, ap.Escalations)
	}
	tr.ingest("audit: carry on", exitOK, upheld(t, sk, "aud-1"), core.StepRunChecks, core.RoleImplementer)

	// Repairing the check changes the candidate, so the claim lapses.
	r.remove("broken")
	c2 := r.commitOnly("repair the check", "broken")
	tr.git("commit c2, remove broken", core.StepFix, core.RoleImplementer)
	_, sk = tr.packet(core.RoleImplementer)
	tr.ingest("fix: claim c1 and c2", exitOK, claim(t, sk, "impl-1", "", c1, c2), core.StepRunChecks, core.RoleImplementer)
	tr.metron(exitOK, core.StepVerify, core.RoleVerifier, "check")

	// The rejected fix is the third failed step: its report raises the
	// process audit, whose discharge suspends the campaign.
	_, sk = tr.packet(core.RoleVerifier)
	d := tr.ingest("verify: rejected", exitOK, verify(t, sk, "ver-1", core.VerificationRejected), core.StepAudit, core.RoleAuditor)
	if !slices.Equal(d.EscalationsRaised, []string{"E-0002"}) || r.state(t).Escalations()[1].Kind != core.EscalationProcessAudit {
		t.Fatalf("the rejection raised %v", d.EscalationsRaised)
	}
	_, sk = tr.packet(core.RoleAuditor)
	d = tr.ingest("audit: carry on", exitOK, upheld(t, sk, "aud-1"), core.StepAdjudicate, core.RoleGovernance)
	if !slices.Equal(d.EscalationsRaised, []string{"E-0003"}) || r.state(t).Escalations()[2].Kind != core.EscalationSuspension {
		t.Fatalf("discharging the process audit raised %v", d.EscalationsRaised)
	}

	// While the campaign is suspended, governance decides and nothing moves.
	ledger := r.read(".metron/state/ledger.jsonl")
	events := r.ledgerEvents()
	tr.metron(exitNegative, core.StepAdjudicate, core.RoleGovernance, "gate")
	res, _ = tr.metron(exitNegative, core.StepAdjudicate, core.RoleGovernance, "packet")
	contains(t, "packet error", res.err, "the next action is adjudicate", "suspended by E-0003")
	res, _ = tr.metron(exitNegative, core.StepAdjudicate, core.RoleGovernance, "check")
	contains(t, "check output", res.out, "The campaign is suspended (E-0003) and records nothing until governance decides")
	// It refuses before running anything: no check result is printed.
	if strings.Contains(res.out, "REQ-1") {
		t.Fatalf("metron check ran a check while the campaign is suspended:\n%s", res.out)
	}
	if d := tr.ingest("review: RED, again", exitOK, first, core.StepAdjudicate, core.RoleGovernance); !d.Duplicate {
		t.Fatalf("ingest document %+v", d)
	}
	if r.ledgerEvents() != events || !bytes.Equal(ledger, r.read(".metron/state/ledger.jsonl")) {
		t.Fatal("a command recorded something while the campaign was suspended")
	}
	// Amending the contract does not resume the campaign: only governance's
	// decision does.
	r.write(".metron/contract.yaml", strings.Replace(campaignContract(2, 2, 3, 20), "An internal demonstration", "A demonstration to the team", 1))
	tr.metron(exitOK, core.StepAdjudicate, core.RoleGovernance, "contract", "pin", "-m", "amend")
	if a := r.next(); !slices.Equal(a.Escalations, []string{"E-0003"}) || a.Command != "" || a.Usage != "metron resolve <escalation> -m <decision>" {
		t.Fatalf("next %+v", a)
	}
	if st := r.status(); st.Suspended != "E-0003" || st.GateOpen || st.FailedStreak != 3 || st.CampaignSteps != 6 || !st.Contract.Pinned {
		t.Fatalf("status: suspended %q, gate open %t, streak %d, steps %d", st.Suspended, st.GateOpen, st.FailedStreak, st.CampaignSteps)
	}

	// Governance resumes it: a new campaign, with nothing counted.
	tr.metron(exitOK, core.StepFix, core.RoleImplementer, "resolve", "E-0003", "-m", "resume")
	if st := r.status(); st.Suspended != "" || st.FailedStreak != 0 || st.CampaignSteps != 0 {
		t.Fatalf("status after the resumption: suspended %q, streak %d, steps %d", st.Suspended, st.FailedStreak, st.CampaignSteps)
	}
	_, sk = tr.packet(core.RoleImplementer)
	c3 := r.change("misspell the greeting once more", "greeting.txt", "hallo\n")
	tr.git("commit c3, hallo", core.StepSubmitReport, core.RoleImplementer)
	tr.ingest("fix: claim c3", exitOK, claim(t, sk, "impl-1", "", c3), core.StepRunChecks, core.RoleImplementer)
	tr.metron(exitOK, core.StepVerify, core.RoleVerifier, "check")
	_, sk = tr.packet(core.RoleVerifier)
	tr.ingest("verify: rejected", exitOK, verify(t, sk, "ver-1", core.VerificationRejected), core.StepFix, core.RoleImplementer)

	// The second rejected fix makes the next one need a root-cause analysis.
	fp, sk := tr.packet(core.RoleImplementer)
	if !slices.Equal(fp.RootCauseRequired, []string{"F-0001"}) {
		t.Fatalf("fix packet root cause required %v", fp.RootCauseRequired)
	}
	c4 := r.change("spell the greeting correctly", "greeting.txt", "hello\n")
	tr.git("commit c4, hello", core.StepSubmitReport, core.RoleImplementer)
	d = tr.ingest("fix: claim c4 without a root cause", exitNegative, claim(t, sk, "impl-1", "", c4), core.StepSubmitReport, core.RoleImplementer)
	if d.Rejection == nil || d.Rejection.Layer != core.LayerSemantic || d.Rejection.Problems[0].Path != "/body/fixes/0/root_cause" {
		t.Fatalf("ingest document %+v", d)
	}
	tr.ingest("fix: claim c4", exitOK, claim(t, sk, "impl-1", "the greeting was typed from memory", c4), core.StepRunChecks, core.RoleImplementer)
	tr.metron(exitOK, core.StepVerify, core.RoleVerifier, "check")
	_, sk = tr.packet(core.RoleVerifier)
	tr.ingest("verify: confirmed", exitOK, verify(t, sk, "ver-1", core.VerificationConfirmed), core.StepReview, core.RoleReviewer)
	_, sk = tr.packet(core.RoleReviewer)
	tr.ingest("review: GREEN", exitOK, review(t, sk, "rev-1", nil), core.StepNone, "")
	tr.metron(exitOK, core.StepNone, "", "gate")
	golden(t, "campaign.suspension.txt", tr.String())
}

// TestCampaignBudget runs a campaign whose failure thresholds are out of
// reach: a review voided by a change to the candidate, a RED round, a dispute
// and the audit that upholds it. The packet that uses up the budget is issued
// together with the suspension; it can still be answered, and nothing is
// issued after it.
func TestCampaignBudget(t *testing.T) {
	r := campaign(t, campaignContract(100, 100, 101, 4))
	tr := &transcript{r: r}
	tr.metron(exitOK, core.StepReview, core.RoleReviewer, "check")
	tr.packet(core.RoleReviewer)
	r.change("spell the greeting correctly", "greeting.txt", "hello\n")
	tr.git("commit hello under the reviewer", core.StepRunChecks, core.RoleImplementer)
	tr.metron(exitOK, core.StepReview, core.RoleReviewer, "check")
	_, sk := tr.packet(core.RoleReviewer)
	tr.ingest("review: RED", exitOK, red(t, sk, "rev-1", misspelled()), core.StepFix, core.RoleImplementer)
	_, sk = tr.packet(core.RoleImplementer)
	tr.ingest("fix: dispute", exitOK, dispute(t, sk, "impl-1"), core.StepAudit, core.RoleAuditor)

	// The fourth packet is the budget: metron packet raises the suspension
	// with it, and the auditor still answers it.
	ap, sk := tr.packet(core.RoleAuditor)
	st := r.status()
	if ap.ID != "P-0004" || st.Suspended != "E-0002" || st.CampaignSteps != 4 || st.CampaignBudget != 4 || st.FailedStreak != 1 {
		t.Fatalf("packet %s; status: suspended %q, steps %d of %d, streak %d", ap.ID, st.Suspended, st.CampaignSteps, st.CampaignBudget, st.FailedStreak)
	}
	if e := r.state(t).Escalations()[1]; e.Kind != core.EscalationSuspension || !strings.Contains(e.Reason, "P-0004") {
		t.Fatalf("escalation %+v", e)
	}
	events, err := store.New(r.path(".metron/state")).Load()
	if err != nil {
		t.Fatal(err)
	}
	if last := events[len(events)-2:]; last[0].Type != core.EventPacketIssued || last[0].More != 1 || last[1].Type != core.EventEscalationRaised {
		t.Fatalf("metron packet wrote %s (more %d) and %s, want the packet and then the suspension, in one batch", last[0].Type, last[0].More, last[1].Type)
	}
	tr.ingest("audit: uphold", exitOK, upheld(t, sk, "aud-1"), core.StepAdjudicate, core.RoleGovernance)
	tr.metron(exitNegative, core.StepAdjudicate, core.RoleGovernance, "packet")
	tr.metron(exitNegative, core.StepAdjudicate, core.RoleGovernance, "check")
	tr.metron(exitNegative, core.StepAdjudicate, core.RoleGovernance, "gate")
	if n := len(r.state(t).Packets()); n != 4 {
		t.Fatalf("%d packets issued, want 4", n)
	}
	tr.metron(exitOK, core.StepFix, core.RoleImplementer, "resolve", "E-0002", "-m", "resume")
	if st := r.status(); st.CampaignSteps != 0 || st.Suspended != "" {
		t.Fatalf("status after the resumption: steps %d, suspended %q", st.CampaignSteps, st.Suspended)
	}
	golden(t, "campaign.budget.txt", tr.String())
}

// TestCampaignGreenUnderSuspension uses up a budget of one packet with a
// review that ends GREEN: the verdict does not approve a suspended campaign,
// and the gate opens once governance resumes it.
func TestCampaignGreenUnderSuspension(t *testing.T) {
	r := campaign(t, campaignContract(100, 100, 101, 1))
	r.change("spell the greeting correctly", "greeting.txt", "hello\n")
	r.must("check")
	_, sk := r.packet()
	if st := r.status(); st.Suspended != "E-0001" {
		t.Fatalf("the only packet of the budget raised no suspension: %+v", st.OpenEscalations)
	}
	r.expectAction(core.StepSubmitReport, core.RoleReviewer)
	r.ingest(exitOK, review(t, sk, "rev-1", nil))
	r.expectAction(core.StepAdjudicate, core.RoleGovernance)
	if g := r.gateDoc(exitNegative); !slices.Equal(failing(g), []string{core.ConditionBlockers}) || !strings.Contains(reasons(g, core.ConditionBlockers), "E-0001") {
		t.Fatalf("gate fails %v; B says %q", failing(g), reasons(g, core.ConditionBlockers))
	}
	r.must("resolve", "E-0001", "-m", "resume")
	r.expectNext(core.StepNone)
	r.must("gate")
}

// TestCampaignStreakSurvivesPins changes the stage and changes it back after
// two failed steps: neither pin erases them, and the third failed step raises
// the convergence analysis.
func TestCampaignStreakSurvivesPins(t *testing.T) {
	r := pinned(t)
	r.must("check")
	_, sk := r.packet()
	r.ingest(exitOK, red(t, sk, "rev-1", misspelled()))
	_, sk = r.packet()
	r.ingest(exitOK, claim(t, sk, "impl-1", "", r.change("misspell the greeting differently", "greeting.txt", "hullo\n")))
	r.must("check")
	_, sk = r.packet()
	r.ingest(exitOK, verify(t, sk, "ver-1", core.VerificationRejected))
	if st := r.status(); st.FailedStreak != 2 {
		t.Fatalf("streak %d after two failed steps", st.FailedStreak)
	}

	r.write(".metron/contract.yaml", stageTwo(testContract))
	r.must("contract", "pin", "-m", "the demonstration becomes a controlled validation")
	if st := r.status(); st.Stage != core.StageControlledValidation || st.FailedStreak != 2 || len(st.Rounds) != 0 {
		t.Fatalf("after the stage change: stage %s, streak %d, rounds %+v", st.Stage, st.FailedStreak, st.Rounds)
	}
	r.write(".metron/contract.yaml", testContract)
	r.must("contract", "pin", "-m", "back to the prototype")
	if st := r.status(); st.Stage != core.StagePrototype || st.FailedStreak != 2 {
		t.Fatalf("after the stage change back: stage %s, streak %d", st.Stage, st.FailedStreak)
	}

	r.expectAction(core.StepFix, core.RoleImplementer)
	_, sk = r.packet()
	r.ingest(exitOK, claim(t, sk, "impl-1", "", r.change("misspell the greeting once more", "greeting.txt", "hallo\n")))
	r.must("check")
	_, sk = r.packet()
	d := r.ingestDoc(exitOK, verify(t, sk, "ver-1", core.VerificationRejected))
	if !slices.Equal(d.EscalationsRaised, []string{"E-0001"}) || r.state(t).Escalations()[0].Kind != core.EscalationNonConvergence {
		t.Fatalf("the third failed step raised %v", d.EscalationsRaised)
	}
	if a := r.expectAction(core.StepAudit, core.RoleAuditor); !slices.Equal(a.Escalations, []string{"E-0001"}) {
		t.Fatalf("next %+v", a)
	}
}

// TestCampaignVerdictsAfterPinningBack approves a candidate, pins another
// contract and then the first one again, unchanged: its GREEN verdict counts
// again, unless the pin in between changed the stage, which is a new question
// for the gate.
func TestCampaignVerdictsAfterPinningBack(t *testing.T) {
	for _, tc := range []struct {
		name, other string
		want        core.Step
		owner       core.Role
	}{
		{"same stage", strings.Replace(testContract, "An internal demonstration", "A demonstration to the team", 1), core.StepNone, ""},
		{"another stage", stageTwo(testContract), core.StepReview, core.RoleReviewer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := green(t)
			r.write(".metron/contract.yaml", tc.other)
			r.must("contract", "pin", "-m", "another contract")
			r.expectAction(core.StepReview, core.RoleReviewer)
			r.write(".metron/contract.yaml", testContract)
			r.must("contract", "pin", "-m", "the first contract again")
			r.expectAction(tc.want, tc.owner)
		})
	}
}

// exhaustion performs the actions metron next names the way roles that
// never converge do: every review is RED, every fix is rejected, every audit
// carries on. After every command it checks that the gate is closed.
type exhaustion struct {
	r *repo
	// attempts counts the fixes, each a new greeting; unlisted holds the
	// commits no accepted claim lists yet.
	attempts int
	unlisted []string
}

// closed fails the test unless metron gate exits 1.
func (x *exhaustion) closed(after string) {
	x.r.t.Helper()
	if g := x.r.run("gate"); g.code != exitNegative {
		x.r.t.Fatalf("after %s, metron gate exits %d:\n%s%s", after, g.code, g.out, g.err)
	}
}

func (x *exhaustion) run(code int, args ...string) {
	x.r.t.Helper()
	x.r.expect(code, args...)
	x.closed("metron " + strings.Join(args, " "))
}

func (x *exhaustion) packet() core.Report {
	x.r.t.Helper()
	_, sk := x.r.packet()
	x.closed("metron packet")
	return sk
}

func (x *exhaustion) ingest(report []byte) {
	x.r.t.Helper()
	x.r.ingest(exitOK, report)
	x.closed("metron ingest")
}

// step performs the next action.
func (x *exhaustion) step() {
	r, t := x.r, x.r.t
	t.Helper()
	switch a := r.next(); a.Step {
	case core.StepRunChecks:
		x.run(exitOK, "check")
	case core.StepReview:
		x.ingest(red(t, x.packet(), "rev-1", misspelled()))
	case core.StepFix:
		sk := x.packet()
		x.attempts++
		x.unlisted = append(x.unlisted, r.change("try again", "greeting.txt", fmt.Sprintf("attempt %d\n", x.attempts)))
		x.closed("a commit")
		x.ingest(claim(t, sk, "impl-1", "analysed", x.unlisted...))
		x.unlisted = nil
	case core.StepVerify:
		x.ingest(verify(t, x.packet(), "ver-1", core.VerificationRejected))
	case core.StepAudit:
		x.ingest(upheld(t, x.packet(), "aud-1"))
	default:
		t.Fatalf("roles that never converge cannot perform %+v", a)
	}
}

// TestCampaignNoApprovalThroughExhaustion runs roles that never converge
// through three campaigns, each ended by a suspension and resumed by
// governance: after every command the gate stays closed, and each campaign
// stops within its budget and within process_audit_after failed steps.
func TestCampaignNoApprovalThroughExhaustion(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		convergence, process, budget int
		// why is what the reason of every suspension says.
		why string
	}{
		{"the budget runs out", 3, 5, 6, "which is its budget"},
		{"the process audit suspends", 2, 3, 20, "process audit E-"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := campaign(t, campaignContract(2, tc.convergence, tc.process, tc.budget))
			x := &exhaustion{r: r}
			for n := 1; n <= 3; n++ {
				for steps := 0; r.next().Owner != core.RoleGovernance; steps++ {
					if steps > 4*tc.budget {
						t.Fatalf("campaign %d: %d actions without a governance-owned one; next is %+v", n, steps, r.next())
					}
					x.step()
				}
				st := r.status()
				if st.Suspended == "" || st.CampaignSteps > tc.budget || st.FailedStreak > tc.process {
					t.Fatalf("campaign %d stopped for governance with suspension %q after %d packets and %d failed steps",
						n, st.Suspended, st.CampaignSteps, st.FailedStreak)
				}
				for _, e := range st.OpenEscalations {
					if e.ID == st.Suspended && !strings.Contains(e.Reason, tc.why) {
						t.Fatalf("campaign %d was suspended because %s", n, e.Reason)
					}
				}
				for a := r.next(); a.Owner == core.RoleGovernance; a = r.next() {
					x.run(exitOK, "resolve", a.Escalations[0], "-m", "resume")
				}
			}
		})
	}
}
