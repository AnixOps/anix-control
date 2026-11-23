### Removed

- The protobuf contracts `anix.pluginhost.v1` (`sdk/api/pluginhost/v1`, the
  kernel-to-host protocol, unused since the kernel calls the built-in hosts
  directly) and `anixops.modulepki.v1` (`sdk/api/modulepki/v1`, network
  module enrollment, never served in production) are retired: their generated
  code and their lines in `contracts/proto/descriptors.golden` are gone, and
  `check_proto_golden.py` lists them as retired. The Agent contract and every
  other contract are unchanged.
