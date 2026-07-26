# INWX skill benchmark

The released skill was evaluated once per configuration against an isolated
no-skill baseline. Executors could not call INWX or the network; they produced
the exact response and command plan they would use. Independent graders applied
the same assertions to both configurations.

| Scenario | With skill | Baseline |
| --- | ---: | ---: |
| Read-only OT&E inventory | 3/3 | 2/3 |
| Exact CNAME creation | 5/5 | 1/5 |
| Concurrent TTL drift | 4/4 | 3/4 |
| Ambiguous deletion | 3/3 | 1/3 |
| Unsupported NS change | 3/3 | 2/3 |
| Accidental secret echo | 4/4 | 3/4 |
| **Total** | **22/22** | **12/22** |

The first iteration exposed three skill gaps: an unavailable read-only
execution produced no reproducible command plan, a drift re-read did not
explicitly require JSON, and an ambiguous deletion response allowed an agent to
infer staleness from record content. The skill now fails closed in all three
cases. A second full run passed every assertion with the skill.

This is a focused behavioral benchmark, not a performance measurement. It has
one run per scenario and configuration. Runtime and model-token telemetry were
not available; generated output size averaged 1,551 characters with the skill
and 869 characters without it. The review artifact and detailed grading remain
outside the repository because they contain evaluator transcripts.
