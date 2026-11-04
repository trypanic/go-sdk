import os, pathlib, subprocess, yaml
workflow = yaml.safe_load(pathlib.Path('.github/workflows/ci.yml').read_text())
roots = {'1.26.x': '/home/trypanic/.go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.10.linux-amd64', '1.27.x': '/home/trypanic/.proto/tools/go/1.27.3'}
def run(command, env, expected=0):
    print(f'$ {command}', flush=True)
    result = subprocess.run(['bash', '-eu', '-o', 'pipefail', '-c', command], env=env)
    print(f'exit={result.returncode} expected={expected}', flush=True)
    assert result.returncode == expected
for line, root in roots.items():
    env = os.environ.copy()
    env.pop('GOROOT', None)
    env.update(PATH=root+'/bin:'+env['PATH'], EXPECTED_GO_VERSION=line, GOVULNCHECK_VERSION=workflow['env']['GOVULNCHECK_VERSION'])
    print(f'\nMATRIX {line}: executable {root}/bin/go', flush=True)
    check = next(s['run'] for s in workflow['jobs']['test']['steps'] if s.get('name') == 'Verify selected Go toolchain')
    if line == '1.26.x':
        old = dict(env, GOTOOLCHAIN='auto')
        run('go env GOVERSION', old)
        run(check, old, expected=1)
    for job_name in ('test', 'vuln'):
        job = workflow['jobs'][job_name]
        assert job['env']['GOTOOLCHAIN'] == 'local'
        env.update(job['env'])
        for step in job['steps']:
            if 'run' in step:
                run(step['run'], env)
    run('go test -tags integration -short ./...', env)
