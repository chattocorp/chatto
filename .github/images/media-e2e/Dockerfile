ARG PLAYWRIGHT_VERSION
FROM mcr.microsoft.com/playwright:v${PLAYWRIGHT_VERSION}-noble

ARG PLAYWRIGHT_VERSION
LABEL org.opencontainers.image.source="https://github.com/chattocorp/chatto"

# Install codec libraries once, when publishing the image, instead of in each job.
RUN apt-get update \
    && apt-get install --no-install-recommends -y ffmpeg git curl ca-certificates unzip zstd \
    && rm -rf /var/lib/apt/lists/* \
    && printf '%s\n' "$PLAYWRIGHT_VERSION" > /opt/media-e2e-playwright-version

