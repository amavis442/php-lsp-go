<?php

declare(strict_types=1);

namespace App\Domain;

use App\Contracts\HasName;

#[\Attribute]
final readonly class Money
{
    public function __construct(
        public int $amount,
        public string $currency = 'EUR',
    ) {}
}

class Account implements HasName
{
    public const string TYPE = 'basic';

    public private(set) int $balance = 0;

    public string $name {
        get => ucfirst($this->name);
        set (string $value) {
            $this->name = trim($value);
        }
    }

    public function deposit(Money $money): void
    {
        $this->balance += $money->amount;
    }
}

enum Status: int
{
    case Active = 1;
    case Closed = 0;

    public function label(): string
    {
        return match ($this) {
            self::Active => 'active',
            self::Closed => 'closed',
        };
    }
}

function make_account(): Account
{
    return new Account();
}
