require 'xcodeproj'
require 'pathname'
root = File.expand_path('..', __dir__)
project_path = File.join(root, 'JobsIM.xcodeproj')
existing_project = File.file?(File.join(project_path, 'project.pbxproj'))
project = existing_project ? Xcodeproj::Project.open(project_path) : Xcodeproj::Project.new(project_path, false, 77)
app = project.targets.find { |target| target.name == 'JobsIM' } || project.new_target(:application, 'JobsIM', :ios, '16.0')
test = project.targets.find { |target| target.name == 'JobsIMTests' } || project.new_target(:unit_test_bundle, 'JobsIMTests', :ios, '16.0')
ui_test = project.targets.find { |target| target.name == 'JobsIMUITests' } || project.new_target(:ui_test_bundle, 'JobsIMUITests', :ios, '16.0')
[test, ui_test].each do |target|
  target.add_dependency(app) unless target.dependencies.any? { |dependency| dependency.target == app }
end

# Migrate only the App's old references; keep Pods integration, tests and existing target UUIDs.
app_root = File.join(root, 'JobsIM') + File::SEPARATOR
project.files.select { |ref| ref.real_path.to_s.start_with?(app_root) }.each(&:remove_from_project)
project.main_group.children.select { |group| group.isa == 'PBXGroup' && group.path == 'JobsIM' }.each do |group|
  group.recursive_children.reverse_each(&:remove_from_project)
  group.remove_from_project
end
app_group = project.main_group.children.find { |group| group.isa == 'PBXFileSystemSynchronizedRootGroup' && group.path == 'JobsIM' }
unless app_group
  app_group = project.new(Xcodeproj::Project::Object::PBXFileSystemSynchronizedRootGroup)
  app_group.path = 'JobsIM'
  app_group.source_tree = '<group>'
  app_group.explicit_file_types = {}
  app_group.explicit_folders = []
  project.main_group.children << app_group
end
exceptions = app_group.exceptions.find { |entry| entry.isa == 'PBXFileSystemSynchronizedBuildFileExceptionSet' && entry.target == app }
unless exceptions
  exceptions = project.new(Xcodeproj::Project::Object::PBXFileSystemSynchronizedBuildFileExceptionSet)
  exceptions.target = app
  app_group.exceptions << exceptions
end
exceptions.membership_exceptions = (Array(exceptions.membership_exceptions) + ['Resource/Info.plist', 'Resource/Simulator.entitlements']).uniq.sort
app.file_system_synchronized_groups << app_group unless app.file_system_synchronized_groups.include?(app_group)
# xcodeproj 1.27 supports synchronized groups but exposes objectVersion as read-only when opening a project.
project.instance_variable_set(:@object_version, '77')
project.root_object.compatibility_version = 'Xcode 16.0'
project.root_object.known_regions = (project.root_object.known_regions + Dir.glob(File.join(root, 'JobsIM/Resource/*.lproj')).map { |folder| File.basename(folder, '.lproj') }).uniq.sort

test_group = project.main_group.groups.find { |group| group.name == 'Tests' } || project.main_group.new_group('Tests', '../../../测试/前端/iOS')
Dir.glob(File.expand_path('../../../测试/前端/iOS/*.swift', root)).sort.each do |file|
  ref = test_group.files.find { |entry| entry.path == File.basename(file) } || test_group.new_file(File.basename(file))
  test.source_build_phase.add_file_reference(ref) unless test.source_build_phase.files_references.include?(ref)
end
Dir.glob(File.expand_path('../../../测试/前端/iOS/Fixtures/*', root)).sort.each do |file|
  path = "Fixtures/#{File.basename(file)}"
  ref = test_group.files.find { |entry| entry.path == path } || test_group.new_file(path)
  test.resources_build_phase.add_file_reference(ref) unless test.resources_build_phase.files_references.include?(ref)
end
ui_group = project.main_group.groups.find { |group| group.name == 'UITests' } || project.main_group.new_group('UITests', '../../../测试/前端/iOS/UI')
Dir.glob(File.expand_path('../../../测试/前端/iOS/UI/*.swift', root)).sort.each do |file|
  ref = ui_group.files.find { |entry| entry.path == File.basename(file) } || ui_group.new_file(File.basename(file))
  ui_test.source_build_phase.add_file_reference(ref) unless ui_test.source_build_phase.files_references.include?(ref)
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
    c.build_settings.delete('CODE_SIGNING_ALLOWED[sdk=iphoneos*]')
    c.build_settings['CODE_SIGN_STYLE'] = 'Automatic'
    c.build_settings['DEVELOPMENT_TEAM'] = 'K5692KLW75'
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
exit if existing_project && File.file?(File.join(project.path, 'xcshareddata/xcschemes/JobsIM.xcscheme'))
scheme = Xcodeproj::XCScheme.new
scheme.add_build_target(app)
scheme.add_test_target(test)
scheme.add_test_target(ui_test)
scheme.set_launch_target(app)
scheme.save_as(project.path, 'JobsIM', true)
