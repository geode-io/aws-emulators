ARG EMULATOR=kinesis-subscription-emulator

FROM golang:1.21 as build
ARG EMULATOR
WORKDIR /emu
COPY . .
RUN make build-emulator EMU=${EMULATOR}

FROM debian:buster-slim
ARG EMULATOR
COPY --from=build /emu/bin/emulator/${EMULATOR} ./emulator
ENTRYPOINT [ "./emulator" ]

