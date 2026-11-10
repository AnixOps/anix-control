### Changed

- A scheduled workflow (`artifact-cleanup.yml`, every six hours) deletes
  GitHub Actions artifacts older than one day. Artifacts only carry files
  between the jobs of one run; release assets are not touched.
