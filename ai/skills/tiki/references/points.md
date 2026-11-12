# Estimating points

`points` is a relative estimate of how big and hard a tiki is. Set it in the `create` statement of
every tiki you create. A tiki without points reads as "not estimated yet", so don't leave it unset
and don't pick a filler value.

Base the estimate on three things, and let the largest one drive it:

- **Effort**: the amount of work, in files, components and layers touched and tests to write.
- **Complexity**: how hard the work is, from design decisions, tricky logic, or care it needs.
- **Uncertainty**: how much is unknown, such as unclear requirements, unfamiliar code, or a bug with
  no known cause. More unknowns mean a higher estimate.

| Points | Size | Typical work |
|---|---|---|
| 1 | trivial | a typo, a config value, a one-line fix with an obvious cause |
| 2–3 | small | a contained change in one or two files with a clear approach |
| 4–5 | medium | several files or one component in depth; a few design decisions; a bug that needs investigation |
| 6–7 | large | a feature across several components, a migration, or work with notable unknowns |
| 8–9 | very large | a cross-cutting change, significant design or research, a high risk of surprises |
| 10 | huge | the largest work the project takes on as one tiki |

The table assumes a scale of 1 to 10. If the schema lists other `points` values, estimate with the
table, then pick the closest listed value, treating the smallest as 1 and the largest as 10.

- Before estimating work on code in the current project, look at the code it touches. A title alone
  is not enough to estimate on.
- Points are relative. Compare with tikis the project has already estimated:
  `tiki exec 'select id, title, points where has(points) order by points'`.
- If a tiki rates 9 or 10 and has parts that could ship separately, suggest splitting it into smaller
  tikis.
- A tiki that only groups other tikis is rated as a whole against other such groups, not as the sum
  of its parts.
- When you start work on a tiki that has no points, estimate it first:
  `tiki exec 'update where id = "X7F4K2" set points="5"'`.
- To find unestimated tikis: `tiki exec 'select id, title where not has(points)'`.
- Don't change an estimate that is already set, unless the user asks or the tiki's scope has changed.
  Tell the user when you change one.
