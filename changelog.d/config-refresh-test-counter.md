### Fixed

- The agent configuration refresh test (`TestAgentControlConfigRefresh`) no
  longer fails when another session's refresher counts a snapshot at the
  same time: it waits for the process-wide refresh counter to move rather
  than to reach an exact value. Test-only; nothing changes for operators.
