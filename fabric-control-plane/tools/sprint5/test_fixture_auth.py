import unittest
from datetime import datetime,timezone,timedelta
from fixture_auth import active_grants

class FixtureAuthTests(unittest.TestCase):
    def test_removes_expired_entry_without_extending_or_changing_active_authority(self):
        now=datetime(2026,11,7,tzinfo=timezone.utc)
        good={'expiresAt':(now+timedelta(minutes=10)).isoformat(),'permissions':['read'],'tokenSHA256':'fixture-hash'}
        expired={'expiresAt':now.isoformat(),'permissions':['cleanup'],'tokenSHA256':'expired-hash'}
        original=[expired,good]
        result=active_grants(original,now)
        self.assertEqual(result,[good]);self.assertIs(result[0],good)
        self.assertEqual(original,[expired,good]);self.assertEqual(good['permissions'],['read'])
        self.assertEqual(good['expiresAt'],(now+timedelta(minutes=10)).isoformat())
    def test_ambiguous_malformed_or_empty_authority_fails_closed(self):
        now=datetime(2026,11,7,tzinfo=timezone.utc)
        for grants in [[],[{'expiresAt':'2026-11-08'}],[{'expiresAt':'invalid'}],[{'expiresAt':now.isoformat()}],[{}]]:
            with self.subTest(grants=grants):
                with self.assertRaises(ValueError):active_grants(grants,now)
        with self.assertRaises(ValueError):active_grants([],datetime(2026,11,7))
if __name__=='__main__':unittest.main()
