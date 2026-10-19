# Resolves conflict hunks by keeping the union of lines (ours first), for list-like files.
import re,sys,json
p=sys.argv[1]; s=open(p).read()
def rep(m):
    out=[]
    for x in (m.group(1)+m.group(2)).split('\n'):
        if x.strip() and x not in out and x.rstrip(',') not in [o.rstrip(',') for o in out]: out.append(x)
    return '\n'.join(out)+'\n'
s=re.sub(r'<<<<<<< [^\n]*\n(.*?)=======\n(.*?)>>>>>>> [^\n]*\n',rep,s,flags=re.S)
if p.endswith('.json'):
    lines=s.split('\n')
    # normalise commas inside the pages array
    s='\n'.join(lines); s=re.sub(r'("[0-9a-f-]{36}")\n',r'\1,\n',s); s=re.sub(r',\n(\s*)\]',r'\n\1]',s)
    json.loads(s)
open(p,'w').write(s)
