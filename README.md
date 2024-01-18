# AWS Emulators

This directory contains a suite of tools for running aws service emulators offline for testing and development.
This includes slim emulator binaries for lambda invocation flows, and localstack configuration for emulation of an AWS environment.
It also includes envoy configuration to emulate the AWS lambda edge (not API gateway).

## Emulators

The goal of these emulators should always be to remain as small as possible to reduce the likelihood of bugs which may cause false positives in local testing.
