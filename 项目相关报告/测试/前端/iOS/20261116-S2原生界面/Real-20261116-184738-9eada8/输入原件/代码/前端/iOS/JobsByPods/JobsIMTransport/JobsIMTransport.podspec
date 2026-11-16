Pod::Spec.new do |s|
  s.name = 'JobsIMTransport'
  s.version = '0.1.0'
  s.summary = 'JobsIM native JobsIMTransport boundary'
  s.homepage = 'https://github.com/JobsKits/JobsIM'
  s.license = { :type => 'Apache-2.0' }
  s.author = { 'Jobs' => 'lg295060456@gmail.com' }
  s.source = { :path => '.' }
  s.ios.deployment_target = '16.0'
  s.swift_version = '6.0'
  s.source_files = 'Core/**/*.swift'
  s.dependency 'JobsIMCore'
  s.dependency 'JobsNetworking/Core'
  s.dependency 'Alamofire'
end
