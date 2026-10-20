#!/usr/bin/env python3
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent
FAKE_SCRIPT = '''#!/usr/bin/env python3
import os,shlex,subprocess,sys
from pathlib import Path
if os.environ.get('SMOKE_CASE')=='management':sys.exit(23)
if os.environ.get('SMOKE_CASE')=='incomplete':print('HTTP/1.1 200 OK');sys.exit(0)
args=shlex.split(sys.argv[sys.argv.index('-c')+1])
assert args[:3]==['az','containerapp','exec']
assert args[args.index('-n')+1]=='fixture-gateway'
# ACA console splits startup command words without shell quote interpretation.
command=args[args.index('--command')+1].split()
sys.exit(subprocess.run(command).returncode)
'''
FAKE_WGET = '''#!/usr/bin/env python3
import json,os,sys
from pathlib import Path
p=Path(os.environ['CASE_DIR']);n=int((p/'count').read_text()) if (p/'count').exists() else 0
(p/'count').write_text(str(n+1))
a=sys.argv[1:];url=a[-1]
code=503 if os.environ.get('SMOKE_CASE')=='mixed' and n==3 else 200
body={'status':'UP'} if '/method/' in url else [{'id':1,'code':'KJV','name':'Fixture','updated_at':1,'locale':'en'}]
if os.environ.get('SMOKE_CASE')=='bad-json' and n==4:body={'error':'not a versions list'}
Path(a[a.index('-O')+1]).write_text(json.dumps(body))
print('  HTTP/1.1 '+str(code)+' status',file=sys.stderr)
sys.exit(0 if code==200 else 8)
'''

class SmokeTests(unittest.TestCase):
    def test_smoke_requires_every_sample_and_structured_body(self):
        for case, want in [('ok',0), ('mixed',1), ('bad-json',1), ('management',2), ('incomplete',2)]:
            with self.subTest(case=case), tempfile.TemporaryDirectory() as directory:
                p=Path(directory)
                for name,body in [('script',FAKE_SCRIPT),('wget',FAKE_WGET),('sleep','#!/bin/sh\nexit 0\n')]:
                    (p/name).write_text(body);(p/name).chmod(0o755)
                env=dict(os.environ, PATH=directory+os.pathsep+os.environ['PATH'], CASE_DIR=directory,
                    RESOURCE_GROUP='fixture', API_GATEWAY_APP_NAME='fixture-gateway', SMOKE_CASE=case)
                proc=subprocess.run(['bash',str(ROOT/'scripts/smoke-release.sh')],env=env,capture_output=True,text=True,timeout=15)
                self.assertEqual(proc.returncode,want,proc.stdout+proc.stderr)
                if case not in ('management','incomplete'):self.assertEqual((p/'count').read_text(),'9')
    def test_smoke_rejects_nonoperational_health_path_before_console(self):
        env=dict(os.environ,RESOURCE_GROUP='fixture',API_GATEWAY_APP_NAME='fixture-gateway',SMOKE_HEALTH_PATH='/bad;echo nope')
        proc=subprocess.run(['bash',str(ROOT/'scripts/smoke-release.sh')],env=env,capture_output=True,text=True,timeout=5)
        self.assertNotEqual(proc.returncode,0)

if __name__=='__main__':unittest.main()
