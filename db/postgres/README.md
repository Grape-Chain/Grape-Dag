# Working with PostreSQL

## Introduction
In your development environment you will need to run an instance of PostgreSQL to allow DAG to be stored on each node. Each node, regardless of its other functional capabilities (at least based on the latests discussions) will run an instance of PostreSQL.

You can start an instance of PostreSQL in a docker container. You can run the following command:

```bash
cd db/postres
docker compose up
```
If you would like to explore your db after it's launched the best way to launch it would be:
```bash
$ docker-compose run postgres bash # drop into the container shelldatabase# 

> psql --host=postgres --username=luna --dbname=lunadb
Password for user luna: 
psql (12.0 (Debian 12.0-2.pgdg100+1))
Type "help" for help.lunadb=#
...
```
_NOTE_: the docker compose file references postgres.env file which contains the key env vars PostgreSQL relies on when starting up

## PostgreSQL Driver and Toolkit
In luna1 project we use __PGX__ driver. [pgx is a pure Go driver and toolkit for PostgreSQL]

The pgx driver is a low-level, high performance interface that exposes PostgreSQL-specific features such as `LISTEN` / `NOTIFY` and `COPY`. It also includes an adapter for the standard `database/sql` interface.

The toolkit component is a related set of packages that implement PostgreSQL functionality such as parsing the wire protocol and type mapping between PostgreSQL and Go. These underlying packages can be used to implement alternative drivers, proxies, load balancers, logical replication clients, etc.

