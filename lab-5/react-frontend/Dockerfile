FROM node:18-alpine AS build

WORKDIR /app

COPY react-frontend/package*.json ./

RUN npm ci

COPY react-frontend/ ./

ENV REACT_APP_API_BASE_URL=${REACT_APP_API_BASE_URL:-http://localhost:3000}

RUN npm run build


FROM node:18-alpine

WORKDIR /app

RUN npm i -g serve
 
COPY --from=build /app/build ./build

EXPOSE 3000

CMD ["serve", "-s", "build", "-l", "3000"]
