#!/usr/bin/env python3
"""Validate the frozen documentation-only plan; not an application test suite.

The todo/owner check intentionally applies to the planning phase. Revise it
explicitly when implementation begins; the inventory records initial states.
"""
import json
from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parent.parent
ERRORS = []


def require(condition, message):
    if not condition:
        ERRORS.append(message)


def section(text, heading):
    match = re.search(r'^## ' + re.escape(heading) + r'\s*\n(.*?)(?=^## |\Z)', text, re.M | re.S)
    return match.group(1).strip() if match else ''


def bullets(text):
    return re.findall(r'^- (.+)$', text, re.M)


def field(text, name):
    match = re.search(r'^' + re.escape(name) + r': (.+)$', text, re.M)
    return match.group(1).strip() if match else ''


def safe_path(value):
    return value and not value.startswith(('/', '~')) and '..' not in Path(value).parts and '\\' not in value


def main():
    tasks = {}
    for path in sorted((ROOT / '.tasks').rglob('*.md')):
        text = path.read_text()
        match = re.match(r'# Task (\d{4}) - (.+)\n', text)
        if not match:
            require(False, f'{path.relative_to(ROOT)}: unrecognized task heading')
            continue
        task_id, title = match.groups()
        require(task_id not in tasks, f'duplicate task ID {task_id}')
        require(path.name.startswith(task_id + '-'), f'task ID/path mismatch {path.name}')
        deps = field(text, 'Depends on')
        tasks[task_id] = dict(path=path, text=text, title=title,
                              deps=[] if deps == 'none' else [d.strip() for d in deps.split(',')])
    expected = {f'{i:04}' for i in range(5, 50)}
    future = {i for i in tasks if int(i) >= 5}
    require(future == expected, f'expected 45 future tasks 0005–0049, got {len(future)}')
    for task_id, task in tasks.items():
        for dep in task['deps']:
            require(dep in tasks, f'{task_id}: missing dependency {dep}')
    visiting, visited = set(), set()

    def visit(task_id):
        if task_id in visiting:
            require(False, f'cyclic dependency at {task_id}')
            return
        if task_id in visited or task_id not in tasks:
            return
        visiting.add(task_id)
        for dep in tasks[task_id]['deps']:
            visit(dep)
        visiting.remove(task_id)
        visited.add(task_id)

    for task_id in tasks:
        visit(task_id)
    for task_id in sorted(future):
        task = tasks[task_id]
        text = task['text']
        require(field(text, 'State') == 'todo' and field(text, 'Owner') == 'none',
                f'{task_id}: future task must be unclaimed todo at planning boundary')
        require(field(text, 'Mode') == 'mvp', f'{task_id}: unexpected mode')
        require(field(text, 'Risk') in ('low', 'medium', 'high', 'critical'), f'{task_id}: missing risk')
        for heading in ('Goal', 'Context files', 'Files allowed to edit', 'Files forbidden to edit',
                        'Steps', 'Acceptance criteria', 'Invariants', 'Review questions',
                        'Counterexample searches', 'Verification', 'Documentation updates', 'Notes'):
            require(bool(section(text, heading)), f'{task_id}: missing {heading}')
        for value in bullets(section(text, 'Context files')):
            require(safe_path(value) and (ROOT / value).is_file(), f'{task_id}: absent/unsafe context {value}')
        allowed = bullets(section(text, 'Files allowed to edit'))
        require(len(allowed) == len(set(allowed)), f'{task_id}: duplicate allowed paths')
        for value in allowed:
            require(safe_path(value), f'{task_id}: unsafe allowed path {value}')
        require('docs/progress.md' in allowed, f'{task_id}: lifecycle progress path absent')
        require(task['path'].relative_to(ROOT).as_posix() in allowed, f'{task_id}: own task path absent')
        checks = []
        for line in bullets(section(text, 'Verification')):
            try:
                checks.append(json.loads(line.strip('`')))
            except (ValueError, TypeError):
                require(False, f'{task_id}: invalid gated verification JSON')
        require(any(c.get('type') == 'automated' and c.get('required') and c.get('command') for c in checks),
                f'{task_id}: no required future command')
        require('none is claimed to pass today' in text, f'{task_id}: missing future-test disclaimer')
        require(len(text.encode()) < 12000, f'{task_id}: unusually large task; review decomposition')
    # Review regressions: final-schema verification must follow the final migration,
    # and the import slice must be permitted to mount its new UI components.
    require('0043' in tasks['0044']['deps'], '0044 must follow final migration task 0043')
    wiring = set(bullets(section(tasks['0043']['text'], 'Files allowed to edit')))
    for value in ('web/src/activity/ActivityPage.tsx', 'web/src/work/WorkPage.tsx',
                  'web/src/api/releases.ts', 'web/src/activity/ActivityPage.test.tsx',
                  'web/src/work/WorkPage.test.tsx'):
        require(value in wiring, f'0043 missing integration path {value}')
    for task_id, value in (('0030', 'web/src/api/client.ts'),
                           ('0033', 'web/src/work/WorkPage.tsx'),
                           ('0034', 'web/src/edition/EditionPage.tsx'),
                           ('0036', 'web/src/activity/ActivityPage.tsx')):
        require(value in section(tasks[task_id]['text'], 'Steps'),
                f'{task_id} must create the named future integration anchor {value}')
    plan = (ROOT / 'docs/implementation-plan.md').read_text()
    inventory = {}
    coverage = {}
    for line in plan.splitlines():
        if line.startswith('| ['):
            cols = [v.strip() for v in line.strip('|').split('|')]
            match = re.match(r'\[(\d{4})\]\((.+)\)', cols[0])
            require(bool(match) and len(cols) == 6, 'malformed inventory row')
            if not match or len(cols) != 6:
                continue
            task_id, link = match.groups()
            require(task_id not in inventory, f'duplicate inventory row {task_id}')
            inventory[task_id] = cols
            task = tasks.get(task_id)
            require(task is not None, f'unknown inventory task {task_id}')
            if task:
                require(cols[1] == task['title'], f'{task_id}: inventory title mismatch')
                require(cols[2].split(', ') == task['deps'], f'{task_id}: inventory dependencies mismatch')
                require(cols[3] == field(task['text'], 'Risk'), f'{task_id}: inventory risk mismatch')
                require(f'Milestone: {cols[4]}.' in task['text'], f'{task_id}: inventory milestone mismatch')
            require(cols[5] == 'todo', f'{task_id}: wrong initial state')
        elif re.match(r'^\| R\d\d \|', line):
            cols = [v.strip() for v in line.strip('|').split('|')]
            require(len(cols) == 4, f'bad coverage row {cols[0]}')
            if len(cols) != 4:
                continue
            require(cols[0] not in coverage, f'duplicate requirement {cols[0]}')
            coverage[cols[0]] = cols
            owners = re.findall(r'\b\d{4}\b', cols[2])
            require(bool(owners) and all(i in future for i in owners), f'{cols[0]}: invalid task owners')
            require(bool(re.fullmatch(r'[A-G](, [A-G])*', cols[3])), f'{cols[0]}: invalid acceptance scenarios')
            require('](' in cols[1], f'{cols[0]}: no design link')
    require(set(inventory) == expected, 'inventory does not cover exactly 45 future tasks')
    required = set(re.findall(r'^\| (R\d\d) \|', (ROOT / 'docs/product/requirements.md').read_text(), re.M))
    require(set(coverage) == required == {f'R{i:02}' for i in range(1, 19)}, 'requirement coverage incomplete')
    owned = {i for cols in coverage.values() for i in re.findall(r'\b\d{4}\b', cols[2])}
    require(owned == future, f'tasks without requirement ownership: {sorted(future-owned)}')
    scenarios = set(re.findall(r'[A-G]', ','.join(cols[3] for cols in coverage.values())))
    require(scenarios == set('ABCDEFG'), 'not all acceptance scenarios covered')
    for path in [ROOT / 'README.md', *sorted((ROOT / 'docs').rglob('*.md'))]:
        for target in re.findall(r'\[[^\]]+\]\(([^)]+)\)', path.read_text()):
            target = target.split('#', 1)[0]
            if not target or re.match(r'^[a-zA-Z]+:', target):
                continue
            destination = (path.parent / unquote(target)).resolve()
            require(destination.is_relative_to(ROOT) and destination.is_file(),
                    f'{path.relative_to(ROOT)}: missing/unsafe local link {target}')
    # Protect historic task files and ADR text from accidental rewrites in this phase.
    baseline = '7acb2f0458bdfff33bd48c4312b2eb2be944b56d'
    for path in sorted((ROOT / '.tasks').glob('000[1-3]-*.md')):
        original = subprocess.check_output(['git', 'show', f'{baseline}:{path.relative_to(ROOT)}'], cwd=ROOT)
        require(path.read_bytes() == original, f'historic task changed: {path.name}')
    prior_decisions = subprocess.check_output(['git', 'show', f'{baseline}:docs/decisions.md'], cwd=ROOT, text=True)
    require((ROOT / 'docs/decisions.md').read_text().startswith(prior_decisions), 'accepted ADR history rewritten')
    if ERRORS:
        print('\n'.join('FAIL: ' + e for e in ERRORS))
        return 1
    print(f'PASS: {len(tasks)} unique task IDs; 45 unclaimed todo tasks; dependency references valid; no cycles.')
    print('PASS: inventory, risk, milestone, context/allowed paths and future verification declarations agree.')
    print('PASS: 18 requirements and scenarios A–G covered; every implementation task has an owner requirement.')
    print('PASS: local documentation links resolve; legacy tasks and accepted ADR history preserved.')
    print('Static planning checks only; no application tests were executed.')
    return 0


if __name__ == '__main__':
    sys.exit(main())
