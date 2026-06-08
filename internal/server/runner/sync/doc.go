// Package sync implements a server.Runner that reconciles Rollout records against the live state on agents:
//  1. Lists actionable rollouts (status Pending, Drift, or retry-eligible Failed).
//  2. Dispatches each by its [enum.RolloutIntent]:
//     - Install    → ApplyTask(spec)                               → save TaskId
//     - Update     → ApplyTask(spec)                               → save new TaskId
//     - Uninstall  → DeleteTask(actualTaskID) (or noop if empty)   → drop rollout
//     - Noop       → skip (safety; filter shouldn't hand these out)
//  3. After processing rollouts, the finalizer pass actually removes any
//     `Spec.DeletionRequested=true` spec whose last rollout has drained.
package sync
