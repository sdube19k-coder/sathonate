"""Fixture configuration maintenance, never an authentication bypass."""
from datetime import datetime

def active_grants(grants,now):
    if now.tzinfo is None or not isinstance(grants,list):
        raise ValueError('Explicit timezone and grant list required')
    remaining=[]
    for grant in grants:
        if not isinstance(grant,dict) or not isinstance(grant.get('expiresAt'),str):
            raise ValueError('Malformed fixture expiry')
        try:expiry=datetime.fromisoformat(grant['expiresAt'])
        except ValueError:raise ValueError('Malformed fixture expiry') from None
        if expiry.tzinfo is None:raise ValueError('Ambiguous fixture expiry')
        if expiry>now:remaining.append(grant)
    if not remaining:raise ValueError('No active fixture grants remain; fresh authentication is required')
    return remaining
