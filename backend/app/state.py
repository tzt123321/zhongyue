# Shared application state - daily recommendation cache
_daily_recommend_cache = {"track": None, "date": None}


def get_daily_recommend_cache():
    return _daily_recommend_cache
