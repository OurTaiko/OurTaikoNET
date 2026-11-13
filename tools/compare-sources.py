#!/usr/bin/env python3
"""How far the two song packages are from being one: every declaration both have, compared.

Run from the repository root. The merge of internal/fanmade and internal/official is done
declaration by declaration (make both texts the same, then keep one); this lists what is left.
"""
import re, glob, difflib, sys
def decls(d):
    out={}
    for f in sorted(glob.glob(d+'/*.go')):
        if f.endswith('_test.go'): continue
        s=open(f).read()
        for m in re.finditer(r'^(func (?:\([^)]*\) )?(\w+)\(|type (\w+) |const (\w+) |var (\w+) )', s, re.M):
            name=m.group(2) or m.group(3) or m.group(4) or m.group(5); start=m.start(); line_end=s.index('\n', start)
            first=s[start:line_end].rstrip()
            if first.endswith('{') or first.endswith('('):
                closer='\n}' if first.endswith('{') else '\n)'
                end=s.index(closer, start)+2
            elif first.endswith('`') and first.count('`')%2==1:
                end=s.index('`', line_end)+1
            else:
                end=line_end
            # the comment block right above belongs to the declaration
            cs=start
            while True:
                prev=s.rfind('\n', 0, cs-1)
                line=s[prev+1:cs-1] if cs>0 else ''
                if line.startswith('//'): cs=prev+1
                else: break
            out[('m:' if m.group(1).startswith('func (') else '')+name]={'text':s[start:end],'doc':s[cs:start],'file':f.split('/')[-1]}
    return out
def code_only(t):
    lines=[]
    for l in t.split('\n'):
        l=re.sub(r'\s+//.*$','',l) if not l.strip().startswith('//') else ''
        if l.strip(): lines.append(l.strip())
    return '\n'.join(lines)
if __name__=='__main__':
    a=decls('internal/fanmade/httpapi'); b=decls('internal/official/httpapi')
    both=set(a)&set(b)
    same=[k for k in both if a[k]['text']==b[k]['text']]
    diff=[k for k in both if a[k]['text']!=b[k]['text']]
    comment=[k for k in diff if code_only(a[k]['text'])==code_only(b[k]['text'])]
    small=[]
    for k in diff:
        if k in comment: continue
        n=sum(1 for l in difflib.unified_diff(code_only(a[k]['text']).split('\n'), code_only(b[k]['text']).split('\n'), lineterm='', n=0) if l[:1] in '+-' and l[:3] not in ('+++','---'))
        small.append((n,k,a[k]['file']))
    small.sort()
    L=lambda ks,src: sum(src[k]['text'].count('\n')+1 for k in ks)
    print(f"in both {len(both)}: identical {len(same)} ({L(same,a)} lines); differ only in comments {len(comment)} ({L(comment,a)} lines); differ in code {len(small)} ({L([k for _,k,_ in small],a)} fanmade lines)")
    print('code-differing, smallest first (changed lines, name, file):')
    for n,k,f in small: print(f"  {n:4} {k:38} {f}")
