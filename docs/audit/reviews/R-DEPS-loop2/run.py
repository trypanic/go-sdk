import json, os, pathlib, subprocess, time
OUT = pathlib.Path('/tmp/DEPS-review2')
ROOT = OUT / 'clone'
def run(name, command, env=None, cwd=ROOT, expected=0):
    start = time.monotonic()
    with (OUT / (name+'.log')).open('w') as f:
        f.write('$ '+command+'\n'); f.flush()
        p = subprocess.run(['bash','-eu','-o','pipefail','-c', command], cwd=cwd, env=env, stdout=f, stderr=subprocess.STDOUT)
    row=dict(name=name, command=command, cwd=str(cwd), exit=p.returncode, expected=expected, seconds=round(time.monotonic()-start,2))
    with (OUT/'checks.jsonl').open('a') as f: f.write(json.dumps(row)+'\n')
    print(json.dumps(row),flush=True)
    return p.returncode
if __name__ == '__main__':
    run('check','task check')
    run('task-test','task test')
    run('tagged-vet','go vet -tags integration ./...')
    run('tagged-staticcheck','staticcheck -tags integration ./...')
    run('tagged-vuln','go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -tags=integration -test ./...')
    run('lint','GOPROXY=file:///home/trypanic/.go/pkg/mod/cache/download GOSUMDB=off task lint')
    run('release-default','task release:check',expected=201)
    run('release-comparison','task release:check RELEASE_VERSION=v2.0.0')
    run('release-initial','task release:check BASE=none RELEASE_VERSION=v2.0.0')
    run('coverage','go test -short -count=1 -coverprofile=/tmp/DEPS-review2/coverage.out ./... && go tool cover -func=/tmp/DEPS-review2/coverage.out')
    run('short-no-docker','DOCKER_HOST=unix:///tmp/DEPS-review2/absent.sock go test -count=1 -tags integration -short ./...')
    run('short-no-docker-verbose','DOCKER_HOST=unix:///tmp/DEPS-review2/absent.sock go test -count=1 -v -tags integration -short -run "Integration|TestBindLoopback" ./postgres ./mongo ./messaging ./internal/integrationtest')
    run('format','gofmt -l .')
    run('status','git status --porcelain')
