#! /usr/bin/env ruby
# frozen_string_literal: true

require 'pg'
require 'sensu-plugin/check/cli'

class CheckPostgres < Sensu::Plugin::Check::CLI
  option :user,
         description: 'Postgres User',
         short: '-u USER',
         long: '--user USER'

  option :password,
         description: 'Postgres Password',
         short: '-p PASS',
         long: '--password PASS'

  option :hostname,
         description: 'Hostname to login to',
         short: '-h HOST',
         long: '--hostname HOST'

  option :database,
         description: 'Database schema to connect to',
         short: '-d DATABASE',
         long: '--database DATABASE'

  option :port,
         description: 'Database port',
         short: '-P PORT',
         long: '--port PORT'

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
    res = con.exec('select version();')
    info = res.first

    ok "Server version: #{info}"
  rescue PG::Error => e
    critical "Error message: #{e.error.split("\n").first}"
  ensure
    con&.close
  end
end
