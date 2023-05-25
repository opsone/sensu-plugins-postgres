# frozen_string_literal: true

require 'bundler/gem_tasks'
require 'rubocop/rake_task'

RuboCop::RakeTask.new

task default: :rubocop

task :build_asset do
  `docker build -t ruby-plugin-debian -f Dockerfile.debian .`
  `docker run -v "$PWD/assets:/tmp/assets" ruby-plugin-debian cp /assets/sensu-plugins-postgres.tar.gz /tmp/assets/sensu-plugins-postgres_#{Sensu::Plugins::Postgres::VERSION}_debian_linux.tar.gz`
  `docker rm $(docker ps -a -q --filter ancestor=ruby-plugin-debian)`
  `docker rmi ruby-plugin-debian`
end
