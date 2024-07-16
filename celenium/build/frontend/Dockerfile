# ---------------------------------------------------------------------
#  The first stage container, for building the application
# ---------------------------------------------------------------------
FROM node:20-alpine AS base
ENV PNPM_HOME="/pnpm"
ENV PATH="$PNPM_HOME:$PATH"
RUN corepack enable

RUN apk --no-cache add git

FROM base as install
WORKDIR /app
RUN git clone https://github.com/distractedm1nd/celenium-interface.git
WORKDIR /app/celenium-interface
RUN --mount=type=cache,id=pnpm,target=/pnpm/store pnpm install --frozen-lockfile

FROM install AS build
COPY --from=install /app/celenium-interface /app/celenium-interface
WORKDIR /app/celenium-interface
RUN ["pnpm", "build"]

FROM build AS serve
COPY --from=build /app/celenium-interface/.output /app/celenium-interface/.output
WORKDIR /app/celenium-interface
ENTRYPOINT ["node", ".output/server/index.mjs"]
