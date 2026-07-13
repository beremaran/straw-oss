# Maintained scripts

This directory contains non-destructive verification commands invoked by `make help` and CI. Each script must be
portable to the supported CI environment, fail with actionable output, and have a discoverable Make target. Product
experiments and one-off infrastructure commands do not belong here.
