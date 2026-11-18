require 'xcodeproj'
require 'pathname'
root = File.expand_path('..', __dir__)
project = Xcodeproj::Project.new(File.join(root, 'JobsIM.xcodeproj'))
app = project.new_target(:application, 'JobsIM', :ios, '16.0')
test = project.new_target(:unit_test_bundle, 'JobsIMTests', :ios, '16.0')
test.add_dependency(app)
ui_test = project.new_target(:ui_test_bundle, 'JobsIMUITests', :ios, '16.0')
ui_test.add_dependency(app)
app_group = project.main_group.new_group('JobsIM', 'JobsIM')
Dir.glob(File.join(root, 'JobsIM', '**', '*.swift')).sort.each do |file|
  ref = app_group.new_file(Pathname.new(file).relative_path_from(Pathname.new(File.join(root, 'JobsIM'))).to_s)
  app.source_build_phase.add_file_reference(ref)
end
resources = app_group.new_group('Resource', 'Resource')
Dir.glob(File.join(root, 'JobsIM', 'Resource', '*.lproj')).sort.each do |folder|
  next unless File.file?(File.join(folder, 'Localizable.strings'))
  lang = File.basename(folder, '.lproj')
  group = resources.children.find { |x| x.isa == 'PBXVariantGroup' && x.name == 'Localizable.strings' }
  group ||= resources.new_variant_group('Localizable.strings')
  ref = group.new_file("#{lang}.lproj/Localizable.strings")
  ref.name = lang
  app.resources_build_phase.add_file_reference(group) unless app.resources_build_phase.files_references.include?(group)
end
assets = resources.new_file('Assets.xcassets')
app.resources_build_phase.add_file_reference(assets)
test_group = project.main_group.new_group('Tests', '../../../测试/前端/iOS')
Dir.glob(File.expand_path('../../../测试/前端/iOS/*.swift', root)).sort.each do |file|
  test.source_build_phase.add_file_reference(test_group.new_file(File.basename(file)))
end
Dir.glob(File.expand_path('../../../测试/前端/iOS/Fixtures/*', root)).sort.each do |file|
  ref = test_group.new_file("Fixtures/#{File.basename(file)}")
  test.resources_build_phase.add_file_reference(ref)
end
ui_group = project.main_group.new_group('UITests', '../../../测试/前端/iOS/UI')
Dir.glob(File.expand_path('../../../测试/前端/iOS/UI/*.swift', root)).sort.each do |file|
  ui_test.source_build_phase.add_file_reference(ui_group.new_file(File.basename(file)))
end
ui_test.build_configurations.each do |c|
  c.build_settings['TEST_TARGET_NAME'] = 'JobsIM'
end
[app, test, ui_test].each do |target|
  target.build_configurations.each do |c|
    c.build_settings['SWIFT_VERSION'] = '6.0'
    c.build_settings['IPHONEOS_DEPLOYMENT_TARGET'] = '16.0'
    c.build_settings['CODE_SIGNING_ALLOWED'] = 'YES'
    c.build_settings['CODE_SIGN_IDENTITY[sdk=iphonesimulator*]'] = '-'
    c.build_settings['CODE_SIGNING_ALLOWED[sdk=iphoneos*]'] = 'NO'
    c.build_settings['PRODUCT_BUNDLE_IDENTIFIER'] = target == app ? 'com.jobs.im' : target == test ? 'com.jobs.im.tests' : 'com.jobs.im.uitests'
    c.build_settings['GENERATE_INFOPLIST_FILE'] = 'YES'
    c.build_settings['TARGETED_DEVICE_FAMILY'] = '1,2'
    c.build_settings['ENABLE_USER_SCRIPT_SANDBOXING'] = 'NO'
  end
end
app.build_configurations.each do |c|
  c.build_settings['INFOPLIST_FILE'] = 'JobsIM/Resource/Info.plist'
  c.build_settings['GENERATE_INFOPLIST_FILE'] = 'NO'
  c.build_settings['ASSETCATALOG_COMPILER_APPICON_NAME'] = ''
  c.build_settings['CODE_SIGN_ENTITLEMENTS[sdk=iphonesimulator*]'] = 'JobsIM/Resource/Simulator.entitlements'
end
test.build_configurations.each do |c|
  c.build_settings['COMPRESS_PNG_FILES'] = 'NO'
  c.build_settings['TEST_HOST'] = '$(BUILT_PRODUCTS_DIR)/JobsIM.app/JobsIM'
  c.build_settings['BUNDLE_LOADER'] = '$(TEST_HOST)'
end
project.save
scheme = Xcodeproj::XCScheme.new
scheme.add_build_target(app)
scheme.add_test_target(test)
scheme.add_test_target(ui_test)
scheme.set_launch_target(app)
scheme.save_as(project.path, 'JobsIM', true)
