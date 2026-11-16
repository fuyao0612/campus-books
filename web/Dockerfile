FROM node:22-bookworm-slim
WORKDIR /app
COPY --chown=node:node . .
ENV NODE_ENV=production
USER node
EXPOSE 5174
CMD ["node", "server.mjs"]
