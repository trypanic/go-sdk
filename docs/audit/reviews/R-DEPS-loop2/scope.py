import hashlib, json, pathlib, subprocess, sys
sys.path.insert(0, '/tmp/DEPS-review2')
from run import OUT, ROOT, run
def git(*args): return subprocess.check_output(['git',*args],cwd=ROOT)
for sha in ['5e57bba','ac8ab8e','ddf7afd']:
    p=OUT/('modules-'+sha);p.mkdir(exist_ok=True)
    for f in ['go.mod','go.sum']: (p/f).write_bytes(git('show',sha+':'+f))
    assert run('modules-'+sha, 'go list -m all',cwd=p)==0
    assert run('graph-'+sha, 'go mod graph',cwd=p)==0
def selected(sha):
    lines=(OUT/('modules-'+sha+'.log')).read_text().splitlines()[2:]
    return dict(l.split(maxsplit=1) for l in lines)
b=selected('5e57bba'); old=selected('ac8ab8e'); a=selected('ddf7afd')
result=dict(added={m:v for m,v in a.items() if m not in b},removed={m:v for m,v in b.items() if m not in a},changed={m:[b[m],v] for m,v in a.items() if m in b and b[m]!=v},loop2_changed=a!=old)
assert not result['removed'] and not result['changed'] and not result['loop2_changed']
files=git('diff','--name-only','--diff-filter=M','5e57bba','ddf7afd','--','*.go').decode().splitlines()
nonmechanical=[]
for f in files:
    before=git('show','5e57bba:'+f).replace(b'github.com/trypanic/go-sdk',b'github.com/trypanic/go-sdk/v2')
    if before!=git('show','ddf7afd:'+f):nonmechanical.append(f)
result.update(existing_go_files=len(files), nonmechanical=nonmechanical)
assert not nonmechanical
assert not git('diff','5e57bba','ddf7afd','--','CHANGELOG.md','.release-please-manifest.json')
assert not git('diff','ac8ab8e','ddf7afd','--','go.sum')
(OUT/'scope.json').write_text(json.dumps(result,indent=2)+'\n')
assert run('normal-closure','go list -deps -test ./...')==0
assert run('tagged-closure','go list -tags integration -deps -test ./...')==0
assert run('packages','go list ./...')==0
normal=(OUT/'normal-closure.log').read_text();tagged=(OUT/'tagged-closure.log').read_text()
assert 'github.com/testcontainers/' not in normal
assert 'golang.org/x/crypto/openpgp' not in normal+tagged
assert 'github.com/testcontainers/testcontainers-go' in tagged
assert run('format-diff','git diff --check 5e57bba..HEAD',expected=2)==2
print(json.dumps(dict(added=len(result['added']),loop2_changed=result['loop2_changed'],mechanical_go_files=len(files))),flush=True)
