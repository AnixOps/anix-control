### Changed

- 组网: intranet names use the character data of Unicode 15.1.0 from a generated table shared with the node kernel: each character takes its simple lowercase mapping (İ becomes i), and characters unassigned in that version become hyphens. A long non-ASCII name drops whole characters until its encoded label fits in 63 bytes, instead of being cut inside the encoding.
