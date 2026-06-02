import yaml, sys

def resolve_property_collisions(obj):
    """ogen pascal-cases JSON property names, so siblings that differ only by
    case (e.g. CalendarEvent's `ListItemId` vs `listItemId`) collide on the same
    Go field name and abort generation. Inject `x-ogen-properties` overrides that
    give the later members distinct Go names while preserving the JSON keys."""
    props = obj.get("properties")
    if not isinstance(props, dict):
        return
    groups = {}
    for key in props:
        groups.setdefault(key.lower(), []).append(key)
    overrides = obj.get("x-ogen-properties") or {}
    for keys in groups.values():
        if len(keys) < 2:
            continue
        for i, key in enumerate(sorted(keys)[1:], start=1):
            if key not in overrides:
                overrides[key] = {"name": key[:1].upper() + key[1:] + str(i)}
    if overrides:
        obj["x-ogen-properties"] = overrides


def walk(obj):
    if isinstance(obj, dict):
        resolve_property_collisions(obj)
        if "type" in obj and isinstance(obj["type"], list):
            types = obj["type"]
            non_null = [t for t in types if t != "null"]
            has_null = "null" in types
            if not non_null:
                # type: ["null"] only — drop type, mark nullable
                del obj["type"]
                if has_null:
                    obj["nullable"] = True
            elif len(non_null) == 1:
                obj["type"] = non_null[0]
                if has_null:
                    obj["nullable"] = True
            else:
                # multi non-null — pick first, mark nullable if applicable
                obj["type"] = non_null[0]
                if has_null:
                    obj["nullable"] = True
        for v in obj.values():
            walk(v)
    elif isinstance(obj, list):
        for v in obj:
            walk(v)

with open(sys.argv[1]) as f:
    spec = yaml.safe_load(f)
if spec.get("openapi", "").startswith("3.1"):
    spec["openapi"] = "3.0.3"
walk(spec)
with open(sys.argv[2], "w") as f:
    yaml.safe_dump(spec, f, sort_keys=False, allow_unicode=True, width=120)
print("ok")
