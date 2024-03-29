docker run \
    --workdir=/srv/workspace \
    --mount type=bind,source=.,target=/srv/workspace \
    ghcr.io/opentofu/opentofu:latest \
    init
