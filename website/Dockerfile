FROM alpine AS parent

RUN apk add git
RUN git clone https://github.com/opentofu/opentofu.org /work

FROM node
COPY --from=parent /work /work
WORKDIR /work
RUN npm i

VOLUME /work/opentofu-repo
EXPOSE 3000

CMD ["npm", "run", "start", "--", "--host","0.0.0.0"]
