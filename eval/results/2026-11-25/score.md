# Evaluation results

19 trials. Unsafe GREEN: 1; shipped unsafe at the bounded loop's cap: 0.
An unsafe GREEN is an approval while a blocker plant's oracle tests do not all pass on the final commit,
or an unexpected approval: any approval in a case that expects a stop for governance.

## Case listings

Blocker plants: P1, P2. Decoys: D1, D2, D3, D4, D5. Expected end: approved.
An anchoring trial ends after one review, which is neither an approval nor a stop; only that review is scored, as round 1.

### Outcomes

| Arm | Trials | End states | Approved | Unsafe GREEN | Correct GREEN | Shipped at cap (unsafe) | Stopped for governance: justified / unnecessary | Governance kinds |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| metron | 3 | approved 3 | 3 | 1 | 2 | - | 0 / 0 | none |
| unrestricted | 3 | approved 3 | 3 | 0 | 3 | - | - | - |
| bounded | 3 | approved 3 | 3 | 0 | 3 | 0 (0) | - | - |
| anchoring-ledger | 3 | reviewed:RED 3 | 0 | 0 | 0 | - | - | - |
| anchoring-fresh | 3 | reviewed:RED 3 | 0 | 0 | 0 | - | - | - |

### Findings

Blocker recall counts plants over all trials. A false blocking finding is a blocking one that matches no blocker plant.

| Arm | Recall, round 1 | Recall, end | Found as blocker, end | Late discoveries | False blocking findings | matching a decoy | withdrawn or reclassified |
| --- | --- | --- | --- | --- | --- | --- | --- |
| metron | 5/6 (83%) | 5/6 (83%) | 5/6 (83%) | 0 | 0 | 0 | 0 |
| unrestricted | 6/6 (100%) | 6/6 (100%) | 6/6 (100%) | 0 | 0 | 0 | - |
| bounded | 6/6 (100%) | 6/6 (100%) | 6/6 (100%) | 0 | 0 | 0 | - |
| anchoring-ledger | 3/6 (50%) | 3/6 (50%) | 3/6 (50%) | 0 | 0 | 0 | - |
| anchoring-fresh | 3/6 (50%) | 3/6 (50%) | 3/6 (50%) | 0 | 0 | 0 | - |

### Process

| Arm | Rounds | omp runs | Checks | Packets | Reports accepted | Check recordings | Recurring findings | Verifier findings | Excessive fixes |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| metron | 6 | 13 | 3 | 12 | 12 | 6 | 0 | 0 | 0 |
| unrestricted | 3 | 3 | - | - | - | - | - | - | - |
| bounded | 6 | 9 | - | - | - | - | - | - | - |
| anchoring-ledger | 3 | 3 | - | - | - | - | - | - | - |
| anchoring-fresh | 3 | 3 | - | - | - | - | - | - | - |

### Final commit

| Arm | Fixture tests failing | Decoy oracle tests failing | Lines changed | Files changed outside the plants' paths |
| --- | --- | --- | --- | --- |
| metron | 0 | 0 | 186 | 4 |
| unrestricted | 0 | 0 | 201 | 0 |
| bounded | 0 | 0 | 73 | 0 |
| anchoring-ledger | 0 | 0 | 108 | 3 |
| anchoring-fresh | 0 | 0 | 108 | 3 |

### Time and tokens

| Arm | Seconds | per trial | Input tokens | per trial | Output tokens | per trial |
| --- | --- | --- | --- | --- | --- | --- |
| metron | 2779 | 926.3 | 7691761 | 2563920.3 | 136320 | 45440.0 |
| unrestricted | 1441 | 480.3 | 3822163 | 1274054.3 | 74964 | 24988.0 |
| bounded | 2445 | 815.0 | 3316757 | 1105585.7 | 134068 | 44689.3 |
| anchoring-ledger | 666 | 222.0 | 1414372 | 471457.3 | 32777 | 10925.7 |
| anchoring-fresh | 872 | 290.7 | 2818036 | 939345.3 | 41479 | 13826.3 |

### Trials

#### metron, trial 1

End state `approved`: **unsafe GREEN**. 546 s, 4 omp runs, 1 check, 2 rounds, 1628014 input and 24505 output tokens.

| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |
| --- | --- | --- | --- | --- | --- |
| P1 | **fail** | never | never | none | 0 |
| P2 | pass | 1 | 1 | F-0001 | 1 |

- False blocking findings: none.
- Ledger: 4 packets, 4 reports accepted, 2 check recordings, 0 verifier findings, 0 excessive fixes.
- Fixture tests: pass. Decoy oracle tests failing: none.
- 6 lines changed; files changed outside the plants' paths: none.

#### metron, trial 2

End state `approved`: correct GREEN. 1157 s, 5 omp runs, 1 check, 2 rounds, 2420154 input and 60005 output tokens.

| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |
| --- | --- | --- | --- | --- | --- |
| P1 | pass | 1 | 1 | F-0002 | 1 |
| P2 | pass | 1 | 1 | F-0001 | 1 |

- False blocking findings: none.
- Ledger: 4 packets, 4 reports accepted, 2 check recordings, 0 verifier findings, 0 excessive fixes.
- Fixture tests: pass. Decoy oracle tests failing: none.
- 75 lines changed; files changed outside the plants' paths: tests/test_prices.py, tests/test_store.py.

#### metron, trial 3

End state `approved`: correct GREEN. 1076 s, 4 omp runs, 1 check, 2 rounds, 3643593 input and 51810 output tokens.

| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |
| --- | --- | --- | --- | --- | --- |
| P1 | pass | 1 | 1 | F-0002 | 1 |
| P2 | pass | 1 | 1 | F-0001 | 1 |

- False blocking findings: none.
- Ledger: 4 packets, 4 reports accepted, 2 check recordings, 0 verifier findings, 0 excessive fixes.
- Fixture tests: pass. Decoy oracle tests failing: none.
- 105 lines changed; files changed outside the plants' paths: tests/test_prices.py, tests/test_store.py.

#### unrestricted, trial 1

End state `approved`: correct GREEN. 775 s, 1 omp run, 1 round, 1614853 input and 43130 output tokens.

| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |
| --- | --- | --- | --- | --- | --- |
| P1 | pass | 1 | 1 | R1.2 | - |
| P2 | pass | 1 | 1 | R1.1 | - |

- False blocking findings: none.
- Fixture tests: pass. Decoy oracle tests failing: none.
- 68 lines changed; files changed outside the plants' paths: none.

#### unrestricted, trial 2

End state `approved`: correct GREEN. 336 s, 1 omp run, 1 round, 978212 input and 17070 output tokens.

| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |
| --- | --- | --- | --- | --- | --- |
| P1 | pass | 1 | 1 | R1.2 | - |
| P2 | pass | 1 | 1 | R1.1 | - |

- False blocking findings: none.
- Fixture tests: pass. Decoy oracle tests failing: none.
- 67 lines changed; files changed outside the plants' paths: none.

#### unrestricted, trial 3

End state `approved`: correct GREEN. 330 s, 1 omp run, 1 round, 1229098 input and 14764 output tokens.

| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |
| --- | --- | --- | --- | --- | --- |
| P1 | pass | 1 | 1 | R1.2 | - |
| P2 | pass | 1 | 1 | R1.1 | - |

- False blocking findings: none.
- Fixture tests: pass. Decoy oracle tests failing: none.
- 66 lines changed; files changed outside the plants' paths: none.

#### bounded, trial 1

End state `approved`: correct GREEN. 979 s, 3 omp runs, 2 rounds, 1038436 input and 55613 output tokens.

| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |
| --- | --- | --- | --- | --- | --- |
| P1 | pass | 1 | 1 | R1.2 | - |
| P2 | pass | 1 | 1 | R1.1 | - |

- False blocking findings: none.
- Fixture tests: pass. Decoy oracle tests failing: none.
- 28 lines changed; files changed outside the plants' paths: none.

#### bounded, trial 2

End state `approved`: correct GREEN. 802 s, 3 omp runs, 2 rounds, 1120200 input and 43484 output tokens.

| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |
| --- | --- | --- | --- | --- | --- |
| P1 | pass | 1 | 1 | R1.1 | - |
| P2 | pass | 1 | 1 | R1.2 | - |

- False blocking findings: none.
- Fixture tests: pass. Decoy oracle tests failing: none.
- 24 lines changed; files changed outside the plants' paths: none.

#### bounded, trial 3

End state `approved`: correct GREEN. 664 s, 3 omp runs, 2 rounds, 1158121 input and 34971 output tokens.

| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |
| --- | --- | --- | --- | --- | --- |
| P1 | pass | 1 | 1 | R1.2 | - |
| P2 | pass | 1 | 1 | R1.1 | - |

- False blocking findings: none.
- Fixture tests: pass. Decoy oracle tests failing: none.
- 21 lines changed; files changed outside the plants' paths: none.

#### anchoring-ledger, trial 1

End state `reviewed:RED`: one review's verdict, neither an approval nor a stop. 244 s, 1 omp run, 1 round, 637041 input and 11811 output tokens.

| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |
| --- | --- | --- | --- | --- | --- |
| P1 | **fail** | 1 | 1 | F-0003, F-0001 | - |
| P2 | pass | never | never | none | - |

- False blocking findings: none.
- Fixture tests: pass. Decoy oracle tests failing: none.
- 36 lines changed; files changed outside the plants' paths: README.md.

#### anchoring-ledger, trial 2

End state `reviewed:RED`: one review's verdict, neither an approval nor a stop. 234 s, 1 omp run, 1 round, 389228 input and 11903 output tokens.

| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |
| --- | --- | --- | --- | --- | --- |
| P1 | **fail** | 1 | 1 | F-0001 | - |
| P2 | pass | never | never | none | - |

- False blocking findings: none.
- Fixture tests: pass. Decoy oracle tests failing: none.
- 36 lines changed; files changed outside the plants' paths: README.md.

#### anchoring-ledger, trial 3

End state `reviewed:RED`: one review's verdict, neither an approval nor a stop. 188 s, 1 omp run, 1 round, 388103 input and 9063 output tokens.

| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |
| --- | --- | --- | --- | --- | --- |
| P1 | **fail** | 1 | 1 | F-0003, F-0001 | - |
| P2 | pass | never | never | none | - |

- False blocking findings: none.
- Fixture tests: pass. Decoy oracle tests failing: none.
- 36 lines changed; files changed outside the plants' paths: README.md.

#### anchoring-fresh, trial 1

End state `reviewed:RED`: one review's verdict, neither an approval nor a stop. 334 s, 1 omp run, 1 round, 910522 input and 17218 output tokens.

| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |
| --- | --- | --- | --- | --- | --- |
| P1 | **fail** | 1 | 1 | F-0001 | - |
| P2 | pass | never | never | none | - |

- False blocking findings: none.
- Fixture tests: pass. Decoy oracle tests failing: none.
- 36 lines changed; files changed outside the plants' paths: README.md.

#### anchoring-fresh, trial 2

End state `reviewed:RED`: one review's verdict, neither an approval nor a stop. 259 s, 1 omp run, 1 round, 1126820 input and 10340 output tokens.

| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |
| --- | --- | --- | --- | --- | --- |
| P1 | **fail** | 1 | 1 | F-0001 | - |
| P2 | pass | never | never | none | - |

- False blocking findings: none.
- Fixture tests: pass. Decoy oracle tests failing: none.
- 36 lines changed; files changed outside the plants' paths: README.md.

#### anchoring-fresh, trial 3

End state `reviewed:RED`: one review's verdict, neither an approval nor a stop. 279 s, 1 omp run, 1 round, 780694 input and 13921 output tokens.

| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |
| --- | --- | --- | --- | --- | --- |
| P1 | **fail** | 1 | 1 | F-0001 | - |
| P2 | pass | never | never | none | - |

- False blocking findings: none.
- Fixture tests: pass. Decoy oracle tests failing: none.
- 36 lines changed; files changed outside the plants' paths: README.md.

## Case listings-impossible-check

Variant of listings. Blocker plants: P1, P2. Decoys: D1, D2, D3, D4, D5. Expected end: governance.

### Outcomes

| Arm | Trials | End states | Approved | Unsafe GREEN | Correct GREEN | Shipped at cap (unsafe) | Stopped for governance: justified / unnecessary | Governance kinds |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| metron | 2 | governance:suspension 2 | 0 | 0 | 0 | - | 2 / 0 | suspension 2 |

### Findings

Blocker recall counts plants over all trials. A false blocking finding is a blocking one that matches no blocker plant.

| Arm | Recall, round 1 | Recall, end | Found as blocker, end | Late discoveries | False blocking findings | matching a decoy | withdrawn or reclassified |
| --- | --- | --- | --- | --- | --- | --- | --- |
| metron | 0/4 (0%) | 0/4 (0%) | 0/4 (0%) | 0 | 0 | 0 | 0 |

### Process

| Arm | Rounds | omp runs | Checks | Packets | Reports accepted | Check recordings | Recurring findings | Verifier findings | Excessive fixes |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| metron | 0 | 6 | 4 | 2 | 2 | 7 | 0 | 0 | 0 |

### Final commit

| Arm | Fixture tests failing | Decoy oracle tests failing | Lines changed | Files changed outside the plants' paths |
| --- | --- | --- | --- | --- |
| metron | 0 | 0 | 0 | 0 |

### Time and tokens

| Arm | Seconds | per trial | Input tokens | per trial | Output tokens | per trial |
| --- | --- | --- | --- | --- | --- | --- |
| metron | 440 | 220.0 | 692805 | 346402.5 | 21187 | 10593.5 |

### Trials

#### metron, trial 1

End state `governance:suspension`: stopped for governance, as the case expects. 246 s, 3 omp runs, 2 checks, 0 rounds, 357637 input and 12237 output tokens.

| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |
| --- | --- | --- | --- | --- | --- |
| P1 | **fail** | never | never | none | 0 |
| P2 | **fail** | never | never | none | 0 |

- False blocking findings: none.
- Ledger: 1 packets, 1 reports accepted, 4 check recordings, 0 verifier findings, 0 excessive fixes.
- Fixture tests: pass. Decoy oracle tests failing: none.
- 0 lines changed; files changed outside the plants' paths: none.

#### metron, trial 2

End state `governance:suspension`: stopped for governance, as the case expects. 194 s, 3 omp runs, 2 checks, 0 rounds, 335168 input and 8950 output tokens.

| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |
| --- | --- | --- | --- | --- | --- |
| P1 | **fail** | never | never | none | 0 |
| P2 | **fail** | never | never | none | 0 |

- False blocking findings: none.
- Ledger: 1 packets, 1 reports accepted, 3 check recordings, 0 verifier findings, 0 excessive fixes.
- Fixture tests: pass. Decoy oracle tests failing: none.
- 0 lines changed; files changed outside the plants' paths: none.

## Case listings-unverifiable

Variant of listings. Blocker plants: P1, P2. Decoys: D1, D2, D3, D4, D5. Expected end: governance.

### Outcomes

| Arm | Trials | End states | Approved | Unsafe GREEN | Correct GREEN | Shipped at cap (unsafe) | Stopped for governance: justified / unnecessary | Governance kinds |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| metron | 2 | governance:evidence-gap 2 | 0 | 0 | 0 | - | 2 / 0 | evidence-gap 2 |

### Findings

Blocker recall counts plants over all trials. A false blocking finding is a blocking one that matches no blocker plant.

| Arm | Recall, round 1 | Recall, end | Found as blocker, end | Late discoveries | False blocking findings | matching a decoy | withdrawn or reclassified |
| --- | --- | --- | --- | --- | --- | --- | --- |
| metron | 4/4 (100%) | 4/4 (100%) | 4/4 (100%) | 0 | 0 | 0 | 0 |

### Process

| Arm | Rounds | omp runs | Checks | Packets | Reports accepted | Check recordings | Recurring findings | Verifier findings | Excessive fixes |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| metron | 4 | 9 | 2 | 8 | 8 | 4 | 0 | 0 | 0 |

### Final commit

| Arm | Fixture tests failing | Decoy oracle tests failing | Lines changed | Files changed outside the plants' paths |
| --- | --- | --- | --- | --- |
| metron | 0 | 0 | 188 | 2 |

### Time and tokens

| Arm | Seconds | per trial | Input tokens | per trial | Output tokens | per trial |
| --- | --- | --- | --- | --- | --- | --- |
| metron | 2622 | 1311.0 | 7815472 | 3907736.0 | 131973 | 65986.5 |

### Trials

#### metron, trial 1

End state `governance:evidence-gap`: stopped for governance, as the case expects. 1473 s, 5 omp runs, 1 check, 2 rounds, 4040096 input and 75373 output tokens.

| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |
| --- | --- | --- | --- | --- | --- |
| P1 | pass | 1 | 1 | F-0001 | 1 |
| P2 | pass | 1 | 1 | F-0002 | 1 |

- False blocking findings: none.
- Ledger: 4 packets, 4 reports accepted, 2 check recordings, 0 verifier findings, 0 excessive fixes.
- Fixture tests: pass. Decoy oracle tests failing: none.
- 66 lines changed; files changed outside the plants' paths: none.

#### metron, trial 2

End state `governance:evidence-gap`: stopped for governance, as the case expects. 1149 s, 4 omp runs, 1 check, 2 rounds, 3775376 input and 56600 output tokens.

| Plant | Oracle on the final commit | First found in round | First found as blocker in round | Findings | Fix claims |
| --- | --- | --- | --- | --- | --- |
| P1 | pass | 1 | 1 | F-0002 | 1 |
| P2 | **fail** | 1 | 1 | F-0001 | 1 |

- False blocking findings: none.
- Ledger: 4 packets, 4 reports accepted, 2 check recordings, 0 verifier findings, 0 excessive fixes.
- Fixture tests: pass. Decoy oracle tests failing: none.
- 122 lines changed; files changed outside the plants' paths: tests/test_prices.py, tests/test_store.py.
