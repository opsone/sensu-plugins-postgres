# PostgreSQL server with pg_amcheck, run by the sensu user through sudo, like a local agent.
FROM postgres:18.1-trixie
RUN apt-get update && apt-get install -y --no-install-recommends sudo && rm -rf /var/lib/apt/lists/* \
 && useradd --system sensu \
 && echo 'sensu ALL=(postgres) NOPASSWD: /usr/lib/postgresql/18/bin/pg_amcheck' > /etc/sudoers.d/sensu \
 && chmod 440 /etc/sudoers.d/sensu
COPY check-postgres-amcheck /usr/local/bin/
