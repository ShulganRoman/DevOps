FROM alpine:latest

RUN apk update && \
    apk add nginx && \
    mkdir /www && \
    chown -R nginx:nginx /www /var/lib/nginx /var/log/nginx

COPY index.html /www/
COPY default.conf /etc/nginx/http.d/default.conf

USER nginx

CMD ["nginx", "-g", "daemon off;"]
