#!/usr/bin/env python3
"""The importer check tolerates editor serialization, never changed notes."""
import copy
import json
import unittest

from import_audio import canonical_json


class AudioJSONComparisonTests(unittest.TestCase):
    def setUp(self):
        self.model = {'version': 2, 'clips': [{'id': 1, 'sfx': True, 'kind': 'ch3', 'lengthTicks': 12, 'events': [
            {'tick': 0, 'durationTicks': 3, 'note': 60, 'soundId': 1, 'priority': 160},
            {'tick': 4, 'durationTicks': 2, 'note': 64, 'soundId': 2, 'priority': 160}]}]}
        self.expected = canonical_json(json.dumps(self.model))

    def test_editor_key_order_whitespace_and_escapes_are_equivalent(self):
        reordered = copy.deepcopy(self.model)
        reordered['clips'][0] = dict(reversed(list(reordered['clips'][0].items())))
        reordered = dict(reversed(list(reordered.items())))
        self.assertEqual(self.expected, canonical_json(json.dumps(reordered, indent=4)))
        self.assertEqual(canonical_json('{"name":"\\u0391"}'), canonical_json('{"name":"Α"}'))

    def test_every_note_field_and_event_order_remain_strict(self):
        for field in ('tick', 'durationTicks', 'note', 'soundId', 'priority'):
            with self.subTest(field=field):
                changed = copy.deepcopy(self.model)
                changed['clips'][0]['events'][0][field] += 1
                self.assertNotEqual(self.expected, canonical_json(json.dumps(changed)))
        changed = copy.deepcopy(self.model)
        changed['clips'][0]['events'].reverse()
        self.assertNotEqual(self.expected, canonical_json(json.dumps(changed)))

    def test_missing_extra_or_type_changed_fields_are_not_normalized(self):
        for change in ('missing', 'extra', 'boolean', 'float'):
            with self.subTest(change=change):
                changed = copy.deepcopy(self.model)
                clip = changed['clips'][0]
                if change == 'missing': del clip['sfx']
                elif change == 'extra': clip['unknown'] = 0
                elif change == 'boolean': clip['sfx'] = 1
                else: clip['lengthTicks'] = 12.0
                self.assertNotEqual(self.expected, canonical_json(json.dumps(changed)))

    def test_ambiguous_or_non_json_values_are_rejected(self):
        for content in ('{"note":60,"note":61}', '{"note":NaN}', '{"note":Infinity}'):
            with self.subTest(content=content), self.assertRaises(ValueError):
                canonical_json(content)


if __name__ == '__main__':
    unittest.main()
