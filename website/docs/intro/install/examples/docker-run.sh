# Init providers plugins
docker run \
    --workdir=/srv/workspace \
    --mount type=bind,source=.,target=/srv/workspace \
    ghcr.io/opentofu/opentofu:latest \
    init

# Creating plan file
docker run \
    --workdir=/srv/workspace \
    --mount type=bind,source=.,target=/srv/workspace \
    ghcr.io/opentofu/opentofu:latest \
    plan -out=main.plan

# Applying plan file
docker run \
    --workdir=/srv/workspace \
    --mount type=bind,source=.,target=/srv/workspace \
    ghcr.io/opentofu/opentofu:latest \
    apply "/srv/workspace/main.plan"