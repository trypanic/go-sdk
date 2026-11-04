import os,subprocess,sys
binary='/home/trypanic/.go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.10.linux-amd64/bin/go'
env=os.environ.copy();env['GOTOOLCHAIN']='auto';env['PATH']=os.path.dirname(binary)+':'+env['PATH']
p=subprocess.run([binary,'run','golang.org/x/vuln/cmd/govulncheck@v1.9.0','-show','verbose','./...'],env=env)
sys.exit(p.returncode)
