### Removed

- The daily `Artifact prune` workflow. `Artifact Cleanup` already deletes
  Actions artifacts older than a day every six hours, so the 3-day prune had
  nothing left to delete.
