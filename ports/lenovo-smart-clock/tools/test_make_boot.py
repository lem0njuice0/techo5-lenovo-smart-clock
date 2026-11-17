import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from verify_diagnostic_boot import HEADER, parse

TOOLS = Path(__file__).resolve().parent


class BuildBootTests(unittest.TestCase):
    def test_root_flag_and_no_overwrite(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d); page=2048
            donor=root/'donor.img'; kernel=root/'kernel.gz';dtb=root/'board.dtb';out=root/'out.img'
            header=HEADER.pack(b'ANDROID!',4,1,3,2,0,3,4,page,0,0,b'',b'console=tty0',b'',b'')
            original=header.ljust(page,b'\0')+b'kern'.ljust(page,b'\0')+b'ram'.ljust(page,b'\0')
            donor.write_bytes(original);kernel.write_bytes(b'\x1f\x8bkernel');dtb.write_bytes(b'\xd0\x0d\xfe\xeddata')
            args=[sys.executable,str(TOOLS/'make_diagnostic_boot.py'),str(donor),str(kernel),str(dtb),str(out),'--experimental-system-b']
            subprocess.run(args,check=True,capture_output=True)
            self.assertIn('techo5.cube.root=system_b',parse(out)['cmdline'].split())
            result=subprocess.run([sys.executable,str(TOOLS/'verify_diagnostic_boot.py'),str(donor),str(out),str(kernel),str(dtb),'--expect-system-b'],capture_output=True)
            self.assertEqual(result.returncode,0,result.stderr)
            raw=bytearray(out.read_bytes())
            fields=list(HEADER.unpack_from(raw))
            fields[12]=b'console=tty0 techo5.cube.root=system_b keep_initramfs unexpected=1'
            raw[:HEADER.size]=HEADER.pack(*fields)
            out.write_bytes(raw)
            altered=subprocess.run([sys.executable,str(TOOLS/'verify_diagnostic_boot.py'),str(donor),str(out),str(kernel),str(dtb),'--expect-system-b'],capture_output=True)
            self.assertNotEqual(altered.returncode,0)
            self.assertIn(b'"command_line_matches": false',altered.stdout)
            self.assertNotEqual(subprocess.run(args,capture_output=True).returncode,0)
            self.assertEqual(donor.read_bytes(),original)


if __name__=='__main__':unittest.main()
