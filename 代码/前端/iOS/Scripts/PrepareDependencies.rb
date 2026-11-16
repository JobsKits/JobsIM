require 'json'
require 'digest'
require 'fileutils'
require 'open3'
require 'pathname'
root = File.expand_path('..', __dir__)
template = ENV.fetch('JOBS_SWIFT_TEMPLATE', File.expand_path('../../../../../JobsBaseConfig/JobsBaseConfig@JobsSwiftBaseConfigDemo', __dir__))
manifest = JSON.parse(File.read(File.join(root, 'Dependencies', 'TemplateThirdPartySources.json')))
manifest.each do |name, item|
  target = File.join(root, 'Vendor', "#{name}-#{item.fetch('version')}")
  next if File.file?(File.join(target, "#{name}.podspec.json"))
  source = File.join(template, 'Pods', name)
  abort "Missing original template dependency: #{name}; set JOBS_SWIFT_TEMPLATE and install its locked Pods first." unless File.directory?(source)
  prefix = Digest::MD5.hexdigest(name)[0, 3].chars
  spec = %w[trunk cocoapods].map do |repo|
    File.join(Dir.home, '.cocoapods', 'repos', repo, 'Specs', *prefix, name, item.fetch('version'), "#{name}.podspec.json")
  end.find { |path| File.file?(path) }
  abort "Missing exact cached podspec: #{name}" unless spec && Digest::SHA256.file(spec).hexdigest == item.fetch('podspecSHA256')
  FileUtils.cp_r(source, target, preserve: true)
  FileUtils.cp(spec, File.join(target, "#{name}.podspec.json"))
end
grd = JSON.parse(File.read(File.join(root, 'Dependencies', 'GRDB.source.json')))
folder = File.join(root, 'Vendor', 'GRDB.swift-7.11.1')
unless File.file?(File.join(folder, 'GRDB.swift.podspec'))
  archive = File.join(root, 'Vendor', 'GRDB.swift-7.11.1.complete.tar.gz')
  unless File.file?(archive) && Digest::SHA256.file(archive).hexdigest == grd.fetch('archiveSHA256')
    abort 'Official archive download failed' unless system('curl', '-fL', '--connect-timeout', '20', '--max-time', '900', grd.fetch('archiveURL'), '-o', archive)
  end
  abort 'Official archive digest mismatch' unless Digest::SHA256.file(archive).hexdigest == grd.fetch('archiveSHA256')
  listing, status = Open3.capture2('tar', '-tzf', archive)
  abort 'Unsafe or incomplete archive' unless status.success? && listing.lines.all? { |name| !name.start_with?('/') && !name.strip.split('/').include?('..') }
  abort 'Cannot extract official archive' unless system('tar', '-xzf', archive, '-C', File.join(root, 'Vendor'))
end
project = File.join(root, 'JobsIM.xcodeproj')
abort 'Project generation failed' unless File.directory?(project) || system('ruby', File.join(__dir__, 'GenerateProject.rb'))
Dir.chdir(root) do
  abort 'CocoaPods installation failed' unless system('pod', 'install')
end
