FROM oven/bun:1.4.0@sha256:5ff609364c049b54eb0ff560ec96319729a972078ef2c755d758f0c6ef89c2d6 AS builder

WORKDIR /build/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY ./web ./
COPY ./VERSION /build/VERSION
RUN DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(cat /build/VERSION) bun run build

FROM golang:1.26.1-alpine@sha256:2389ebfa5b7f43eeafbd6be0c3700cc46690ef842ad962f6c5bd6be49ed82039 AS builder2
ENV GO111MODULE=on CGO_ENABLED=0 GOWORK=off

ARG TARGETOS
ARG TARGETARCH
ENV GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64}
ENV GOEXPERIMENT=greenteagc

WORKDIR /build

ADD go.mod go.sum ./
# relaykit and tokenkit are local submodules referenced via replace; their
# go.mod files must be present for go mod download to resolve the main module graph.
ADD relaykit/go.mod ./relaykit/go.mod
ADD tokenkit/go.mod ./tokenkit/go.mod
RUN go mod download

COPY . .
COPY --from=builder /build/web/dist ./web/dist
RUN go build -pgo="$(go list -m -f '{{.Dir}}' github.com/Calcium-Ion/moejs)/default.pgo" -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=$(cat VERSION)'" -o new-api

FROM debian:bookworm-slim@sha256:f06537653ac770703bc45b4b113475bd402f451e85223f0f2837acbf89ab020a

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata libasan8 wget \
    && rm -rf /var/lib/apt/lists/* \
    && update-ca-certificates

# Noto Sans TC/SC (regular, TrueType) for CJK text in balance-ledger PDF
# exports. The PDF writer needs TrueType outlines, which the CFF-based
# fonts-noto-cjk package lacks. Set PDF_FONT_DIR to load them from elsewhere.
ARG NOTO_SANS_TC_URL=https://fonts.gstatic.com/s/notosanstc/v40/-nFuOG829Oofr2wohFbTp9ifNAn722rq0MXz76Cy_Co.ttf
ARG NOTO_SANS_TC_SHA256=619662a0583f38311e92666927e5edbfd30f2a1fbe8593685660bd11bdd46a10
ARG NOTO_SANS_SC_URL=https://fonts.gstatic.com/s/notosanssc/v41/k3kCo84MPvpLmixcA63oeAL7Iqp5IZJF9bmaG9_FnYw.ttf
ARG NOTO_SANS_SC_SHA256=450625c8d46ab3df97b7904ded955ec2746d17ec76740cb1e91d1ba63a0f89af
RUN mkdir -p /usr/share/fonts/truetype/noto \
    && wget -q -O /usr/share/fonts/truetype/noto/NotoSansTC.ttf "$NOTO_SANS_TC_URL" \
    && wget -q -O /usr/share/fonts/truetype/noto/NotoSansSC.ttf "$NOTO_SANS_SC_URL" \
    && echo "$NOTO_SANS_TC_SHA256  /usr/share/fonts/truetype/noto/NotoSansTC.ttf" | sha256sum -c - \
    && echo "$NOTO_SANS_SC_SHA256  /usr/share/fonts/truetype/noto/NotoSansSC.ttf" | sha256sum -c -

COPY --from=builder2 /build/new-api /
COPY LICENSE NOTICE THIRD-PARTY-LICENSES.md /licenses/
EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/new-api"]
