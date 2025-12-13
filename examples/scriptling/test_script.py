#!/usr/bin/env scriptling
# Example script demonstrating on-demand library loading

import sys
import testlib

print("Running test script")
print(f"Arguments: {sys.argv}")
print(f"Library function result: {testlib.hello()}")
