import builtins
from src.core import Allan

def test_allan_run(capsys):
    Allan().run()
    captured = capsys.readouterr()
    assert "🤖 Allan готов к работе." in captured.out

