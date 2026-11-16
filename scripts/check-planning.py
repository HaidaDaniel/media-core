#!/usr/bin/env python3
"""Validate the current task graph, contracts and documentation links.

This is a structural check of the implemented repository, not an application
test suite and not a claim about runtime behavior. APK owns task lifecycle and
evidence; this script checks that contracts stay well formed and that the
inventory, requirement coverage and local links agree with them.
"""
import json
from pathlib import Path
import re
import sys
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parent.parent
ERRORS = []
VALID_STATES = {'todo', 'doing', 'review', 'done', 'blocked', 'canceled'}


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
    for task_id, task in tasks.items():
        if int(task_id) < 5:
            continue
        text = task['text']
        state = field(text, 'State')
        require(state in VALID_STATES, f'{task_id}: invalid state {state!r}')
        owner = field(text, 'Owner')
        if state in ('doing', 'review'):
            require(owner != 'none', f'{task_id}: {state} task has no owner')
        require(field(text, 'Mode') == 'mvp' or state == 'canceled' or int(task_id) < 5,
                f'{task_id}: unexpected mode')
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
                f'{task_id}: no required automated command')
        if state == 'todo':
            require('none is claimed to pass today' in text, f'{task_id}: missing future-test disclaimer')
        require(len(text.encode()) < 12000, f'{task_id}: unusually large task; review decomposition')
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
    future = {i for i in tasks if int(i) >= 5}
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
                inventory_deps = [] if cols[2] == 'none' else cols[2].split(', ')
                require(inventory_deps == task['deps'], f'{task_id}: inventory dependencies mismatch')
                require(cols[3] == field(task['text'], 'Risk'), f'{task_id}: inventory risk mismatch')
                require(f'Milestone: {cols[4]}.' in task['text'], f'{task_id}: inventory milestone mismatch')
            require(cols[5] == 'todo', f'{task_id}: wrong initial state')
            require((ROOT / 'docs' / link).is_file(), f'{task_id}: missing inventory link target {link}')
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
    require(set(inventory) == future, 'inventory does not cover exactly the implemented task set')
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
    if ERRORS:
        print('\n'.join('FAIL: ' + e for e in ERRORS))
        return 1
    print(f'PASS: {len(tasks)} unique task IDs; dependency references valid; no cycles.')
    print(f'PASS: inventory covers {len(inventory)} tasks with matching titles, deps, risk and milestones.')
    print('PASS: 18 requirements and scenarios A-G covered; every implementation task has an owner requirement.')
    print('PASS: local documentation links resolve; task contracts are structurally complete.')
    print('Static structural checks only; no application tests were executed.')
    return 0


if __name__ == '__main__':
    sys.exit(main())
