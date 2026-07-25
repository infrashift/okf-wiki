"""Golden-test fixture module."""

import os.path
from collections import defaultdict


class Animal:
    """Base creature."""

    def speak(self):
        """Make a noise."""
        return "..."


class Dog(Animal):
    """A loyal creature."""

    def speak(self):
        return self.bark()

    def bark(self):
        return "woof"


def kennel():
    """Build a default kennel."""
    dogs = defaultdict(list)
    dogs["yard"].append(Dog())
    return os.path.join("home", "kennel"), dogs
