import os, pathlib, sys, yaml
sys.path.insert(0, '/tmp/DEPS-review2')
from run import ROOT, run
w = yaml.safe_load((ROOT/'.github/workflows/ci.yml').read_text())
roots = {'1.26.x':'/home/trypanic/.go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.10.linux-amd64', '1.27.x':'/home/trypanic/.proto/tools/go/1.27.3'}
for line, root in roots.items():
    env = dict(os.environ)
    env.pop('GOROOT',None)
    env.update(PATH=root+'/bin:'+env['PATH'], EXPECTED_GO_VERSION=line, GOVULNCHECK_VERSION=w['env']['GOVULNCHECK_VERSION'])
    for jobname in ['test','vuln']:
        job = w['jobs'][jobname]
        assert job['env']['GOTOOLCHAIN']=='local'
        env.update(job['env'])
        for i, step in enumerate(job['steps']):
            if 'run' not in step: continue
            name = 'matrix-'+line+'-'+jobname+'-'+str(i)
            if line=='1.26.x' and step['name']=='Verify selected Go toolchain':
                assert run(name+'-negative',step['run'],dict(env,GOTOOLCHAIN='auto'),expected=1)==1
            assert run(name,step['run'],env)==0
    assert run('matrix-'+line+'-tagged-short','go test -count=1 -tags integration -short ./...',env)==0
