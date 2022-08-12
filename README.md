# go-rmadison

Read all the metadata from the archive and index them in a DB to allow for easy retrival.

```
go build -o . ./...
./rmadison-server
```

Then to query, via the client:

```
./rmadison linux-azure
```

directly via http:

```
curl http://HOST:PORT/PACKAGE_NAME
```
