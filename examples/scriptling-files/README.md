# knot.files examples

Scripts showing file storage from scriptling. They run anywhere the knot
libraries are available: inside a space through the scriptling CLI with the
knot plugin, or standalone with `KNOT_URL` and `KNOT_TOKEN` set.

    # inside a space, or on the desktop with the knot plugin:
    scriptling --plugin knot write-read.py

    # standalone, configured by environment:
    KNOT_URL=https://knot.example.com KNOT_TOKEN=tk_... scriptling write-read.py

The bucket every script works in comes from `KNOT_FILES_BUCKET`, defaulting
to `scriptfiles` (it is created when missing). Files these scripts write are
ordinary bucket files: `knot file copy`, rclone and the S3 API read them,
and anything those tools upload the scripts can read.
