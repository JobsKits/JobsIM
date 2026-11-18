#!/usr/bin/env python3
"""External Go tests use an ephemeral overlay; no tests are copied into production."""
import argparse,json,os,pathlib,subprocess,tempfile
root=pathlib.Path(__file__).resolve().parents[2]
parser=argparse.ArgumentParser();parser.add_argument('--go',default=os.environ.get('GO_BINARY','go'));parser.add_argument('--backend-only',action='store_true',help='仅运行全部后端外置测试，不重复运行CLI');args=parser.parse_args()
with tempfile.TemporaryDirectory(prefix='jobsim-go-tests-') as directory:
 overlay=pathlib.Path(directory)/'overlay.json'
 # Discover the owned external test set so later feature slices cannot silently
 # disappear from the default regression. Database tests retain explicit gates.
 tests=[root/'测试/后端/security_test.go',*sorted((root/'测试/后端/接口').glob('*_test.go'))]
 mappings={str(root/'代码/后端/Go'/test.name):str(test) for test in tests}
 if len(mappings)!=len(tests):raise RuntimeError('外置测试文件名重复，拒绝覆盖overlay映射')
 print(f'后端回归：{len(tests)}份外置测试；真实数据库用例仍需显式隔离环境。',flush=True)
 overlay.write_text(json.dumps({'Replace':mappings}))
 subprocess.run([args.go,'-C',str(root/'代码/后端/Go'),'test','-overlay',str(overlay),'./...'],check=True)
 if not args.backend_only:
  mappings={str(root/'代码/前端/CLI/cmd/jobsim/outbox_test.go'):str(root/'测试/CLI/outbox_test.go')}
  overlay.write_text(json.dumps({'Replace':mappings}))
  subprocess.run([args.go,'-C',str(root/'代码/前端/CLI'),'test','-overlay',str(overlay),'./...'],check=True)
