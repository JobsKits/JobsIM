require 'json'
require 'open3'
require 'fileutils'
require 'securerandom'
root = File.expand_path('..', __dir__)
mode = ARGV.delete('--test') ? 'test' : 'build'
only = ARGV.select { |value| value.start_with?("--only=") }
only.each { |value| ARGV.delete(value) }
device = ARGV.first
unless device
  text, status = Open3.capture2('xcrun', 'simctl', 'list', 'devices', 'available', '--json')
  abort 'Cannot list simulators' unless status.success?
  devices = JSON.parse(text).fetch('devices').values.flatten
  preferred = mode == 'test' ? 'JobsIM Instance Trust' : 'JobsIM Native'
  device = devices.find { |item| item['name'] == preferred }&.fetch('udid')
  abort "Pass an available simulator UUID or create #{preferred} first." unless device
end
build = File.join(root, 'build')
FileUtils.mkdir_p(build)
command = ['xcodebuild', '-workspace', File.join(root, 'JobsIM.xcworkspace'), '-scheme', 'JobsIM', '-destination', "platform=iOS Simulator,id=#{device}", '-derivedDataPath', File.join(build, 'DerivedData'), 'CODE_SIGN_IDENTITY=-', mode]
if mode == 'test'
  results = File.join(build, 'Results')
  FileUtils.mkdir_p(results)
  result = File.join(results, Time.now.strftime('Test-%Y.%m.%d_%H-%M-%S-') + SecureRandom.hex(4) + '.xcresult')
  command += ['-resultBundlePath', result]
end
only.each { |filter| command << "-only-testing:#{filter.delete_prefix('--only=')}" }
log = File.join(build, "#{mode}.log")
File.open(log, 'w') do |file|
  file.sync = true
  Open3.popen2e(*command) do |input, output, process|
    input.close
    output.each do |line|
      file.write(line)
      puts line if line.include?('error:') || (line.include?('warning:') && line.include?('/JobsIM/JobsIM/')) || line.include?('** ') || line.include?('Test Case')
    end
    exit process.value.exitstatus
  end
end
