#!/usr/bin/env ruby
# frozen_string_literal: true

require 'pg'
require 'sensu-plugin/check/cli'

class CheckPostgresConnections < Sensu::Plugin::Check::CLI
  option :user,
         description: 'Postgres User',
         short: '-u USER',
         long: '--user USER'

  option :password,
         description: 'Postgres Password',
         short: '-p PASS',
         long: '--password PASS'

  option :hostname,
         description: 'Postgres Hostname',
         short: '-h HOST',
         long: '--hostname HOST'

  option :port,
         description: 'Database port',
         short: '-P PORT',
         long: '--port PORT'

  option :database,
         description: 'Database name',
         short: '-d DB',
         long: '--db DB'

  option :warning,
         description: 'Warning threshold number or % of connections. (default: 200 connections)',
         short: '-w WARNING',
         long: '--warning WARNING',
         default: 200,
         proc: proc(&:to_i)

  option :critical,
         description: 'Critical threshold number or % of connections. (default: 250 connections)',
         short: '-c CRITICAL',
         long: '--critical CRITICAL',
         default: 250,
         proc: proc(&:to_i)

  option :use_percentage,
         description: 'Use percentage of max connections used instead of the absolute number of connections',
         short: '-a',
         long: '--percentage',
         boolean: true,
         default: false

  option :timeout,
         description: 'Connection timeout (seconds)',
         short: '-T TIMEOUT',
         long: '--timeout TIMEOUT',
         default: 5,
         proc: proc(&:to_i)

  def run
    con = PG.connect(host: config[:hostname],
                     dbname: config[:database],
                     user: config[:user],
                     password: config[:password],
                     port: config[:port],
                     connect_timeout: config[:timeout])

    max_conns = con.exec('SHOW max_connections').getvalue(0, 0).to_i
    superuser_conns = con.exec('SHOW superuser_reserved_connections').getvalue(0, 0).to_i
    current_conns = con.exec('SELECT count(*) from pg_stat_activity').getvalue(0, 0).to_i

    available_conns = max_conns - superuser_conns
    percent = (current_conns / max_conns.to_f * 100).to_i

    if config[:use_percentage]
      message = "PostgreSQL connections at #{percent}%, #{current_conns} out of #{available_conns} connections"
      if percent >= config[:critical]
        critical message
      elsif percent >= config[:warning]
        warning message
      else
        ok "PostgreSQL connections under threshold: #{percent}%, #{current_conns} out of #{available_conns} connections"
      end
    else
      message = "PostgreSQL connections at #{current_conns} out of #{available_conns} connections"
      if current_conns >= config[:critical]
        critical message
      elsif current_conns >= config[:warning]
        warning message
      else
        ok "PostgreSQL connections under threshold: #{current_conns} out of #{available_conns} connections"
      end
    end
  rescue PG::Error => e
    unknown "Unable to query PostgreSQL: #{e.message}"
  ensure
    con&.close
  end
end
