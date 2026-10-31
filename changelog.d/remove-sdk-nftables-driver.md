### Removed

- The forward SDK's nftables driver (`sdk/forward/driver/nftables`), its nft
  golden scripts (`contracts/forward/v1/nft`) and the netns end-to-end suite
  (`sdk/forward/e2e`) are deleted, together with the "Forward Netns E2E" job and
  the nftables kernel check in Backend Tests. Nothing imports them any more: the
  Agent retires nftables forwarding and relays run on the anixops driver.
