# basher package manifest.
#
# BINS is an explicit list of what basher puts on the user's PATH. Without it
# basher globs bin/* — every file in bin/, executable or not — so a helper
# script or a stray build artifact landing there would silently become a
# published command. Naming the one entry point keeps that from happening.
BINS=bin/gloncher
