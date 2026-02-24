Run the integration tests:
```shell
container_id=$(docker run --rm --name opentofu-psql -p 5432:5432 -e POSTGRES_PASSWORD=tofu -d postgres)
TF_ACC=1 TF_PG_TEST=1 DATABASE_URL="postgresql://postgres:tofu@localhost:5432/postgres?sslmode=disable" \
  go test github.com/opentofu/opentofu/internal/backend/remote-state/pg
docker rm ${container_id} -f
```