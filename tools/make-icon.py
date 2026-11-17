#!/usr/bin/env python3
"""Rasterize Blink's original shield mark to a multi-resolution Windows icon."""
from pathlib import Path
import math
import struct
import zlib

ROOT = Path(__file__).resolve().parent.parent
SHIELD = [(50,17),(77,28),(76,51),(70,66),(61,76),(50,83),(39,76),(30,66),(24,51),(23,28),(50,17)]
CHECK = [(36,48),(46,58),(65,38)]

def distance(x,y,a,b):
    vx,vy=b[0]-a[0],b[1]-a[1]
    t=max(0,min(1,((x-a[0])*vx+(y-a[1])*vy)/(vx*vx+vy*vy)))
    return math.hypot(x-a[0]-t*vx,y-a[1]-t*vy)

def pixel(x,y):
    edge=math.hypot(max(abs(x-50)-33,0),max(abs(y-50)-33,0))
    if edge>15:return (0,0,0,0)
    color=(12,23,36,255)
    if min(distance(x,y,a,b) for a,b in zip(SHIELD,SHIELD[1:]))<1.55 or min(distance(x,y,a,b) for a,b in zip(CHECK,CHECK[1:]))<1.65:
        color=(147,232,209,255)
    return color

def png(n):
    raw=bytearray()
    for y in range(n):
        raw.append(0)
        for x in range(n):
            samples=[pixel((x+(sx+.5)/3)*100/n,(y+(sy+.5)/3)*100/n) for sy in range(3) for sx in range(3)]
            raw.extend(sum(c[i] for c in samples)//9 for i in range(4))
    def chunk(kind,data):return struct.pack('>I',len(data))+kind+data+struct.pack('>I',zlib.crc32(kind+data)&0xffffffff)
    return b'\x89PNG\r\n\x1a\n'+chunk(b'IHDR',struct.pack('>IIBBBBB',n,n,8,6,0,0,0))+chunk(b'IDAT',zlib.compress(bytes(raw),9))+chunk(b'IEND',b'')

def main():
    sizes=[16,32,48,64,256];images=[png(n) for n in sizes];offset=6+16*len(images);entries=[]
    for n,data in zip(sizes,images):
        entries.append(struct.pack('<BBBBHHII',n%256,n%256,0,0,1,32,len(data),offset));offset+=len(data)
    (ROOT/'assets/blink.ico').write_bytes(struct.pack('<HHH',0,1,len(images))+b''.join(entries)+b''.join(images))
    (ROOT/'assets/blink.png').write_bytes(images[-1])

if __name__=='__main__':main()
