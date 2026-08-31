// Package skill resolves the skills a configuration installs into a trial's
// workspace, whether they live on this machine or in a repository somewhere
// else.
//
// A remote skill is a reproducibility problem before it is a download problem.
// mohae exists to run the same configuration twice and compare the results, so
// a source that means one thing today and another next week would quietly turn
// two runs into two different measurements. Everything here is built around
// that: a source is resolved to an immutable commit before it is fetched, the
// cache is keyed by that commit rather than by the text that asked for it, and
// the commit is recorded in the trial result so a run can be reproduced from
// its own report.
package skill
