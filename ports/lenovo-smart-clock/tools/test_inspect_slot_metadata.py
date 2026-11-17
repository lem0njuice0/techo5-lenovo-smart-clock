import struct
import unittest
import zlib
from inspect_slot_metadata import inspect


def record():
    b = bytearray(b'\x00AB0\x01\x00\x00\x00' + bytes([14, 0, 1, 0, 15, 6, 0, 0]) + bytes(12))
    return bytes(b) + struct.pack('>I', zlib.crc32(b))


class SlotTests(unittest.TestCase):
    def test_valid_record_and_misc(self):
        r = record()
        self.assertTrue(inspect(r)['slots']['a']['successful'])
        self.assertEqual(inspect(bytes(0x800)+r)['slots']['b']['tries_remaining'], 6)
        self.assertFalse(inspect(r)['slots']['b']['successful'])

    def test_bad_crc_and_truncation(self):
        b = bytearray(record()); b[12] ^= 1
        for data in (bytes(b), record()[:-1], bytes(32)):
            with self.assertRaises(ValueError):
                inspect(data)


if __name__ == '__main__':
    unittest.main()
