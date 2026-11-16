#!/usr/bin/env python3
"""External Go tests use an ephemeral overlay; no tests are copied into production."""
import argparse,json,os,pathlib,subprocess,tempfile
root=pathlib.Path(__file__).resolve().parents[2]
parser=argparse.ArgumentParser();parser.add_argument('--go',default=os.environ.get('GO_BINARY','go'));args=parser.parse_args()
with tempfile.TemporaryDirectory(prefix='jobsim-go-tests-') as directory:
 overlay=pathlib.Path(directory)/'overlay.json'
 mappings={str(root/'代码/后端/Go/security_test.go'):str(root/'测试/后端/security_test.go'),str(root/'代码/后端/Go/avatar_codec_test.go'):str(root/'测试/后端/接口/avatar_codec_test.go'),str(root/'代码/后端/Go/fresh_migrations_test.go'):str(root/'测试/后端/接口/fresh_migrations_test.go'),str(root/'代码/后端/Go/export_fence_test.go'):str(root/'测试/后端/接口/export_fence_test.go'),str(root/'代码/后端/Go/send_admission_test.go'):str(root/'测试/后端/接口/send_admission_test.go')}
 mappings[str(root/'代码/后端/Go/search_test.go')]=str(root/'测试/后端/接口/search_test.go')
 mappings[str(root/'代码/后端/Go/forward_gateway_test.go')]=str(root/'测试/后端/接口/forward_gateway_test.go')
 mappings[str(root/'代码/后端/Go/session_refresh_test.go')]=str(root/'测试/后端/接口/session_refresh_test.go')
 overlay.write_text(json.dumps({'Replace':mappings}))
 subprocess.run([args.go,'-C',str(root/'代码/后端/Go'),'test','-overlay',str(overlay),'./...'],check=True)
 mappings={str(root/'代码/前端/CLI/cmd/jobsim/outbox_test.go'):str(root/'测试/CLI/outbox_test.go')}
 overlay.write_text(json.dumps({'Replace':mappings}))
 subprocess.run([args.go,'-C',str(root/'代码/前端/CLI'),'test','-overlay',str(overlay),'./...'],check=True)
