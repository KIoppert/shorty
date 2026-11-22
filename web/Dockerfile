FROM nginx:1.29-alpine
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY public /usr/share/nginx/html
RUN chmod -R a+rX /usr/share/nginx/html
